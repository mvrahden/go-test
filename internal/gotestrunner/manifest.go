package gotestrunner

import (
	"encoding/json"
	"slices"
	"sync"
	"time"

	"github.com/mvrahden/go-test/internal/protocol"
)

// runTally is what a stream its manifest opened has held: when it opened,
// each package's last verdict and every test verdict. Nil counts nothing.
type runTally struct {
	mu       sync.Mutex
	opened   time.Time
	packages map[string]string
	// elapsed and hashes are each package's last verdict's time and input
	// hash.
	elapsed map[string]float64
	hashes  map[string]string
	order   []string
	tests   protocol.Tally
	notRun  *int
	vacuous *int
	closed  bool
	// failed are the units of every fail, once each, in stream order.
	failed []protocol.FailedUnit
}

// count books one event of the stream; the run's own package is not counted.
func (r *runTally) count(pkg, test, action string, elapsed float64, hash string) {
	if r == nil || pkg == protocol.RunPackage {
		return
	}
	switch action {
	case "pass", "fail", "skip":
	default:
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if action == "fail" {
		if u := failedUnit(pkg, test); !slices.Contains(r.failed, u) {
			r.failed = append(r.failed, u)
		}
	}
	if test != "" {
		r.tests.Add(action)
		return
	}
	if _, ok := r.packages[pkg]; !ok {
		r.order = append(r.order, pkg)
	}
	r.packages[pkg] = action
	r.elapsed[pkg] = elapsed
	r.hashes[pkg] = hash
}

// verdicts are the packages' last verdicts in stream order, those the run
// booked for itself left out.
func (r *runTally) verdicts(runLevel map[string]bool) []protocol.PackageVerdict {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []protocol.PackageVerdict
	for _, pkg := range r.order {
		if !runLevel[pkg] {
			out = append(out, protocol.PackageVerdict{Package: pkg, Verdict: r.packages[pkg], InputHash: r.hashes[pkg], Elapsed: r.elapsed[pkg]})
		}
	}
	return out
}

// open writes the stream's first event, the run's start carrying m, and
// starts the tally.
func (s *startWriter) open(m *protocol.Manifest) {
	now := time.Now()
	s.run = &runTally{opened: now, packages: map[string]string{}, elapsed: map[string]float64{}, hashes: map[string]string{}}
	s.runLevel[protocol.RunPackage] = true
	data, _ := json.Marshal(runEvent{Time: now, Action: "start", Package: protocol.RunPackage, RunLevel: true, Manifest: m})
	_, _ = s.Write(append(data, '\n'))
}

// runEvent is an event of the run's own package, its fields in go test's
// order.
type runEvent struct {
	Time     time.Time           `json:"Time"`
	Action   string              `json:"Action"`
	Package  string              `json:"Package"`
	Elapsed  *float64            `json:"Elapsed,omitempty"`
	RunLevel bool                `json:"RunLevel"`
	Manifest *protocol.Manifest  `json:"Manifest,omitempty"`
	Result   *protocol.RunResult `json:"Result,omitempty"`
}

// end writes the stream's last event, the run's verdict with code and the
// tally, once; a stream without a manifest gets none. The caller holds the
// gate.
func (s *startWriter) end(code int) {
	r := s.run
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	r.closed = true
	result := r.resultLocked()
	result.ExitCode = code
	opened := r.opened
	r.mu.Unlock()
	action := "pass"
	if code != 0 {
		action = "fail"
	}
	now := time.Now()
	elapsed := now.Sub(opened).Seconds()
	data, _ := json.Marshal(runEvent{Time: now, Action: action, Package: protocol.RunPackage, Elapsed: &elapsed, RunLevel: true, Result: &result})
	s.writeLine(append(data, '\n'))
}

// resultLocked is what the stream has held so far, its exit code aside.
func (r *runTally) resultLocked() protocol.RunResult {
	result := protocol.RunResult{Tests: new(r.tests), NotRun: r.notRun, Vacuous: r.vacuous}
	for _, pkg := range r.order {
		result.Packages.Add(r.packages[pkg])
	}
	return result
}

// snapshot is what the stream has held so far and the units it failed,
// the packages the run booked for itself left out.
func (r *runTally) snapshot(runLevel map[string]bool) (protocol.RunResult, []protocol.FailedUnit) {
	r.mu.Lock()
	defer r.mu.Unlock()
	failed := slices.DeleteFunc(slices.Clone(r.failed), func(u protocol.FailedUnit) bool { return runLevel[u.Package] })
	return r.resultLocked(), failed
}

// judged records what the census found: n declared units without a verdict.
func (s *startWriter) judged(n int) {
	if s == nil || s.run == nil {
		return
	}
	s.run.mu.Lock()
	defer s.run.mu.Unlock()
	s.run.notRun = &n
}

// vacuity records how many tests passed without an assertion.
func (s *startWriter) vacuity(n int) {
	if s == nil || s.run == nil {
		return
	}
	s.run.mu.Lock()
	defer s.run.mu.Unlock()
	s.run.vacuous = &n
}

// EndRun closes the run's -json stream with its exit code: the last event,
// written once. A run whose stream has no manifest, or has none, gets none.
func EndRun(code int) {
	s := liveStream.Load()
	if s == nil {
		return
	}
	s.gate.mu.RLock()
	defer s.gate.mu.RUnlock()
	s.end(code)
}
