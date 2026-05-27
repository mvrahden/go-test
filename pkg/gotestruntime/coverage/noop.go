//go:build gotest_no_coverage_intercept

package coverage

// InterceptTeardown is a no-op when coverage intercept is disabled.
func InterceptTeardown() func() { return func() {} }

// FlushCoverage is a no-op when coverage intercept is disabled.
func FlushCoverage() {}
