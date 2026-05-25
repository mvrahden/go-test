//go:build !gotest_no_coverage_intercept

package coverage

import (
	"testing"
	_ "unsafe"
)

//go:linkname coverReport testing.coverReport
func coverReport()

// InterceptTeardown swaps testing's coverage tearDown with a no-op
// and returns a restore function. If coverage mode is off, returns a no-op.
func InterceptTeardown() func() {
	if testing.CoverMode() == "" {
		return func() {}
	}
	orig := testingCover.tearDown
	testingCover.tearDown = func(string, string) (string, error) { return "", nil }
	return func() {
		testingCover.tearDown = orig
		coverReport()
	}
}

// FlushCoverage writes coverage data. No-op if coverage is disabled.
func FlushCoverage() {
	if testing.CoverMode() == "" {
		return
	}
	coverReport()
}
