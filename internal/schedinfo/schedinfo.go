// Package schedinfo renders one line of best-effort scheduling context for
// budget-verdict failures. A wall-clock verdict taken on a starved machine is
// indistinguishable from a real hang without this: the smoking gun is a
// runnable-but-unscheduled goroutine, and that only shows up in a full dump.
// Everything here is diagnosis-only — read on failure paths, never able to
// fail anything itself, stdlib-only.
package schedinfo

import (
	"fmt"
	"os"
	"runtime"
	"runtime/metrics"
	"strings"
	"time"
)

// starvedLatency is the p99 scheduling latency above which the hint fires.
// Healthy schedulers sit in the microseconds; 100ms of time-to-run means the
// machine, not the code, was the bottleneck.
const starvedLatency = 100 * time.Millisecond

// Summary returns a single parenthesized line of scheduling context, e.g.
// "(gomaxprocs=6 goroutines=124 loadavg=11.42 sched-p50=1ms sched-p99=1.2s)".
// Fields that cannot be read on this platform are omitted.
func Summary() string {
	parts := []string{
		fmt.Sprintf("gomaxprocs=%d", runtime.GOMAXPROCS(0)),
		fmt.Sprintf("goroutines=%d", runtime.NumGoroutine()),
	}
	if load := loadAvg(); load != "" {
		parts = append(parts, "loadavg="+load)
	}
	if p50, p99, ok := schedLatency(); ok {
		parts = append(parts,
			"sched-p50="+p50.Round(time.Microsecond).String(),
			"sched-p99="+p99.Round(time.Microsecond).String())
	}
	return "(" + strings.Join(parts, " ") + ")"
}

// StarvationHint returns a one-sentence hint when the process-lifetime p99
// scheduling latency is pathological, and "" otherwise. Cumulative, not
// windowed — so it suggests, never proves.
func StarvationHint() string {
	if _, p99, ok := schedLatency(); ok && p99 >= starvedLatency {
		return "high scheduler latency suggests CPU starvation, not a hang"
	}
	return ""
}

// loadAvg reads the 1-minute load average, best-effort (Linux only).
func loadAvg() string {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return ""
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// schedLatency reads /sched/latencies:seconds — how long goroutines waited
// in a runnable state before running, cumulative over the process lifetime.
func schedLatency() (p50, p99 time.Duration, ok bool) {
	const name = "/sched/latencies:seconds"
	sample := []metrics.Sample{{Name: name}}
	metrics.Read(sample)
	if sample[0].Value.Kind() != metrics.KindFloat64Histogram {
		return 0, 0, false
	}
	h := sample[0].Value.Float64Histogram()
	p50s, p99s, hok := histogramPercentiles(h.Counts, h.Buckets, 0.50, 0.99)
	if !hok {
		return 0, 0, false
	}
	return time.Duration(p50s * float64(time.Second)), time.Duration(p99s * float64(time.Second)), true
}

// histogramPercentiles resolves two percentiles from a runtime/metrics-shaped
// histogram: counts[i] falls into (buckets[i], buckets[i+1]]. Each percentile
// reports its bucket's upper bound — an honest over-estimate.
func histogramPercentiles(counts []uint64, buckets []float64, q1, q2 float64) (v1, v2 float64, ok bool) {
	var total uint64
	for _, c := range counts {
		total += c
	}
	if total == 0 {
		return 0, 0, false
	}
	resolve := func(q float64) float64 {
		target := uint64(q * float64(total))
		var seen uint64
		for i, c := range counts {
			seen += c
			if seen > target {
				upper := buckets[i+1]
				if upper > 1e18 { // +Inf bucket: fall back to its lower bound
					upper = buckets[i]
				}
				return upper
			}
		}
		return buckets[len(buckets)-1]
	}
	return resolve(q1), resolve(q2), true
}
