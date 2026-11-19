package gotestrunner

import "slices"

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
