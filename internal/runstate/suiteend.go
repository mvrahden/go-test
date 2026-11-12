package runstate

import "sync"

// What other packages have to say when a suite ran whole and passed.
var (
	suiteEndMu sync.Mutex
	suiteEnd   []func(suite string) []string
)

// AtSuiteEnd registers fn to be asked for its notes on a suite, by the
// top-level test's name, once that suite ran whole and passed.
func AtSuiteEnd(fn func(suite string) (notes []string)) {
	suiteEndMu.Lock()
	defer suiteEndMu.Unlock()
	suiteEnd = append(suiteEnd, fn)
}

// SuiteEnded returns every registered function's notes on suite, in the
// order they were registered.
func SuiteEnded(suite string) (notes []string) {
	suiteEndMu.Lock()
	fns := append([]func(string) []string(nil), suiteEnd...)
	suiteEndMu.Unlock()
	for _, fn := range fns {
		notes = append(notes, fn(suite)...)
	}
	return notes
}
