package gotestbench

import (
	"fmt"
	"math"
	"sort"
)

// minSamplesForTTest is the minimum sample count required on both sides of
// a comparison before Welch's t-test is trusted; below that, Compare falls
// back to a plain percent-change heuristic (see Delta.InsufficientSample).
const minSamplesForTTest = 4

// significanceLevel is the two-tailed p-value threshold below which a
// Welch's t-test result is considered significant.
const significanceLevel = 0.05

// insufficientSampleThresholdPct is the documented fallback heuristic used
// when either side has fewer than minSamplesForTTest samples: a benchmark
// is flagged as significant if its mean changed by at least this many
// percent, since a proper significance test isn't trustworthy with so few
// samples.
const insufficientSampleThresholdPct = 20.0

// Delta is the comparison result for one benchmark that exists in both
// baselines.
type Delta struct {
	Key                string  // "pkg Suite/Name"
	OldNs, NewNs       float64 // means
	PercentChange      float64
	Significant        bool // Welch's t-test, p < 0.05; requires >=4 samples each side
	InsufficientSample bool
}

// Compare matches results in old and new by (Package, Suite, Name) and
// returns one Delta per benchmark present in both baselines. Benchmarks
// present in only one baseline are omitted. The returned slice is sorted
// by Key for deterministic output.
func Compare(old, new Baseline) []Delta {
	oldIndex := make(map[string]Result, len(old.Results))
	for _, r := range old.Results {
		oldIndex[resultKey(r)] = r
	}

	deltas := make([]Delta, 0, len(new.Results))
	for _, nr := range new.Results {
		key := resultKey(nr)
		or, ok := oldIndex[key]
		if !ok {
			continue
		}
		deltas = append(deltas, compareResult(key, or, nr))
	}

	sort.Slice(deltas, func(i, j int) bool { return deltas[i].Key < deltas[j].Key })
	return deltas
}

func resultKey(r Result) string {
	return fmt.Sprintf("%s %s/%s", r.Package, r.Suite, r.Name)
}

func compareResult(key string, old, new Result) Delta {
	oldNs := nsSamples(old.Samples)
	newNs := nsSamples(new.Samples)

	oldMean, _ := meanVariance(oldNs)
	newMean, _ := meanVariance(newNs)

	var pctChange float64
	if oldMean != 0 {
		pctChange = (newMean - oldMean) / oldMean * 100
	}

	d := Delta{
		Key:           key,
		OldNs:         oldMean,
		NewNs:         newMean,
		PercentChange: pctChange,
	}

	if len(oldNs) < minSamplesForTTest || len(newNs) < minSamplesForTTest {
		d.InsufficientSample = true
		d.Significant = math.Abs(pctChange) >= insufficientSampleThresholdPct
		return d
	}

	p := welchPValue(oldNs, newNs)
	d.Significant = p < significanceLevel
	return d
}

// WorstRegression returns the largest significant positive PercentChange
// across deltas, or 0 if none of the significant deltas are regressions
// (positive change, i.e. slower).
func WorstRegression(deltas []Delta) float64 {
	worst := 0.0
	for _, d := range deltas {
		if d.Significant && d.PercentChange > worst {
			worst = d.PercentChange
		}
	}
	return worst
}

func nsSamples(samples []Sample) []float64 {
	ns := make([]float64, len(samples))
	for i, s := range samples {
		ns[i] = s.NsPerOp
	}
	return ns
}

// meanVariance returns the sample mean and unbiased (n-1) sample variance
// of xs. Callers with len(xs) < 2 get a variance of 0.
func meanVariance(xs []float64) (mean, variance float64) {
	n := float64(len(xs))
	if n == 0 {
		return 0, 0
	}
	var sum float64
	for _, x := range xs {
		sum += x
	}
	mean = sum / n
	if n < 2 {
		return mean, 0
	}
	var ss float64
	for _, x := range xs {
		d := x - mean
		ss += d * d
	}
	variance = ss / (n - 1)
	return mean, variance
}

// welchPValue computes the two-tailed p-value of Welch's t-test comparing
// the means of a and b, allowing unequal variances and sample sizes.
//
//   - t statistic: (mean(a) - mean(b)) / sqrt(var(a)/n_a + var(b)/n_b)
//   - degrees of freedom: Welch–Satterthwaite equation
//   - p-value: the regularized incomplete beta function evaluated via
//     I_x(df/2, 1/2) with x = df/(df+t^2), the standard closed form for the
//     two-tailed Student's t CDF (Numerical Recipes in C, 3rd ed., §6.4).
func welchPValue(a, b []float64) float64 {
	na, nb := float64(len(a)), float64(len(b))
	meanA, varA := meanVariance(a)
	meanB, varB := meanVariance(b)

	seA := varA / na
	seB := varB / nb
	se2 := seA + seB
	if se2 <= 0 {
		if meanA == meanB {
			return 1
		}
		return 0
	}

	t := (meanA - meanB) / math.Sqrt(se2)
	df := (se2 * se2) / (seA*seA/(na-1) + seB*seB/(nb-1))
	if df <= 0 || math.IsNaN(df) {
		return 1
	}

	x := df / (df + t*t)
	return incompleteBeta(df/2, 0.5, x)
}

// incompleteBeta returns the regularized incomplete beta function I_x(a, b)
// via the continued-fraction expansion, following Numerical Recipes in C,
// 3rd ed., §6.4 ("Incomplete Beta Function") equations 6.4.1-6.4.6: the
// continued fraction converges quickly for x < (a+1)/(a+b+2), and the
// symmetry relation I_x(a,b) = 1 - I_{1-x}(b,a) is used otherwise.
func incompleteBeta(a, b, x float64) float64 {
	if x <= 0 {
		return 0
	}
	if x >= 1 {
		return 1
	}

	lnBeta, _ := math.Lgamma(a + b)
	lnA, _ := math.Lgamma(a)
	lnB, _ := math.Lgamma(b)
	lnBt := lnBeta - lnA - lnB + a*math.Log(x) + b*math.Log(1-x)
	bt := math.Exp(lnBt)

	if x < (a+1)/(a+b+2) {
		return bt * betaContinuedFraction(a, b, x) / a
	}
	return 1 - bt*betaContinuedFraction(b, a, 1-x)/b
}

// betaContinuedFraction evaluates the continued fraction in the incomplete
// beta function via the modified Lentz algorithm (Numerical Recipes in C,
// 3rd ed., §6.4, function betacf).
func betaContinuedFraction(a, b, x float64) float64 {
	const (
		maxIterations = 200
		epsilon       = 3e-7
		tiny          = 1e-30
	)

	qab := a + b
	qap := a + 1
	qam := a - 1

	c := 1.0
	d := 1 - qab*x/qap
	if math.Abs(d) < tiny {
		d = tiny
	}
	d = 1 / d
	h := d

	for m := 1; m <= maxIterations; m++ {
		m2 := float64(2 * m)

		aa := float64(m) * (b - float64(m)) * x / ((qam + m2) * (a + m2))
		d = 1 + aa*d
		if math.Abs(d) < tiny {
			d = tiny
		}
		c = 1 + aa/c
		if math.Abs(c) < tiny {
			c = tiny
		}
		d = 1 / d
		h *= d * c

		aa = -(a + float64(m)) * (qab + float64(m)) * x / ((a + m2) * (qap + m2))
		d = 1 + aa*d
		if math.Abs(d) < tiny {
			d = tiny
		}
		c = 1 + aa/c
		if math.Abs(c) < tiny {
			c = tiny
		}
		d = 1 / d
		del := d * c
		h *= del

		if math.Abs(del-1) < epsilon {
			break
		}
	}

	return h
}
