package gotestrunner

import (
	"context"
	"fmt"
	"os"
	"slices"
)

// dispatchEvent is one step of a run's dispatch.
type dispatchEvent struct {
	Kind      string   // intake, up-front, start, suite-start, suite-end, given-up, stopped, free, teardown
	Package   string   // intake and suite events
	Suite     string   // test function name, suite events
	Group     string   // suite events
	Exclusive bool     // suite events
	Keys      []string // sorted: the fixtures started or freed, or the ones a suite reads
	For       string   // start: what the fixtures start for; intake: "failed" for a build failure
}

// observeDispatch receives every step of a run's dispatch; nil outside tests.
var observeDispatch func(dispatchEvent)

// traceDispatch hands e to the observer when one is set.
func traceDispatch(e dispatchEvent) { //nolint:gocritic // hugeParam: tests only
	if observeDispatch != nil {
		observeDispatch(e)
	}
}

// traceSuite traces a suite event of t, which reads keys.
func traceSuite(kind string, t *SuiteTarget, keys []string) {
	if observeDispatch == nil {
		return
	}
	traceDispatch(dispatchEvent{Kind: kind, Package: t.Package, Suite: t.SuiteName, Group: t.Group, Exclusive: t.Exclusive, Keys: sortedCopy(keys)})
}

func sortedCopy(keys []string) []string {
	out := slices.Clone(keys)
	slices.Sort(out)
	return out
}

// streamRun is one streaming run: what its phases share.
type streamRun struct {
	ctx       context.Context // the stream's: ends at a stop and when the run is over
	cancel    context.CancelFunc
	cfg       PipelineConfig
	pf        ParsedFlags
	overlay   *OverlayResult
	plan      *RunPlan
	scopes    []suiteScope
	collector *OutputCollector
	baseEnv   []string
	sem       chan struct{}
	fixtures  *fixtureSession
	ledger    *fixtureLedger
	releaser  *fixtureReleaser
	clock     groupClock
}

// suiteMode is how a suite gets to run.
type suiteMode int

const (
	streamed suiteMode = iota // beside the compile: waits for its fixtures, then takes a slot
	deferred                  // a later group's bulk: its fixtures were started for it, takes a slot
	alone                     // an exclusive suite: no slot, nothing else runs
)

// runSuite runs one suite and books exactly one verdict for it: its
// result, stopped, or given up for a fixture that did not come up.
func (r *streamRun) runSuite(t SuiteTarget, idx int, mode suiteMode) { //nolint:gocritic // hugeParam: a copy per suite is nothing beside the process it starts
	keys := plannedKeys(r.plan, &t)
	stopped := func() {
		traceSuite("stopped", &t, keys)
		r.collector.RecordStopped(t.Package, idx, t.SuiteName)
	}
	if r.ctx.Err() != nil {
		stopped()
		return
	}
	var missing string
	if mode == streamed {
		var ok bool
		if missing, ok = r.fixtures.await(r.ctx, keys); !ok {
			stopped()
			return
		}
	} else {
		missing = r.fixtures.missing(keys)
	}
	if missing != "" {
		traceSuite("given-up", &t, keys)
		r.collector.RecordGivenUp(t.Package, idx, t.SuiteName, neverRan(t.SuiteName, missing))
		return
	}
	env, err := r.fixtures.env(r.baseEnv, t.Package, t.SuiteName, keys)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s %s: shared fixture state: %s\n", t.Package, t.SuiteName, err)
		r.collector.RecordResult(t.Package, idx, SuiteResult{ExitCode: 1})
		return
	}
	if mode != alone {
		release, ok := r.slot()
		if !ok {
			stopped()
			return
		}
		defer release()
	}
	r.clock.run(t.Group, func() {
		traceSuite("suite-start", &t, keys)
		runRecorded(r.ctx, r.collector, t.Package, idx, t, env, r.collector.UsesTest2JSON())
		traceSuite("suite-end", &t, keys)
	})
}

// slot takes a dispatch slot for a suite, or reports that the run ended
// first; release gives it back.
func (r *streamRun) slot() (release func(), ok bool) {
	select {
	case r.sem <- struct{}{}:
		return func() { <-r.sem }, true
	case <-r.ctx.Done():
		return nil, false
	}
}

// done tells the ledger the suite's process is gone and queues what that frees.
func (r *streamRun) done(t *SuiteTarget) {
	r.free(r.ledger.Done(t.Package, t.SuiteName))
}

// free queues keys no planned suite left needs for release.
func (r *streamRun) free(keys []string) {
	if len(keys) > 0 {
		traceDispatch(dispatchEvent{Kind: "free", Keys: sortedCopy(keys)})
	}
	r.releaser.Release(keys)
}
