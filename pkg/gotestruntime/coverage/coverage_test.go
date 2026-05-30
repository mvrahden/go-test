//go:build !gotest_no_coverage_intercept

package coverage //nolint:stdlib-test

import (
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

func TestInterceptTeardown_NoCoverageMode(t *testing.T) {
	// When run without -cover, CoverMode() is empty.
	// InterceptTeardown should return a callable no-op.
	restore := InterceptTeardown()
	gotest.NotEqual(t, nil, restore)
	restore() // must not panic
}

func TestFlushCoverage_NoCoverageMode(t *testing.T) {
	// When run without -cover, FlushCoverage should be a no-op.
	FlushCoverage() // must not panic
}

func TestInterceptTeardown_WithCoverage(t *testing.T) {
	if testing.CoverMode() == "" {
		t.Skip("test requires -cover flag")
	}

	// Verify the tearDown is swapped
	origTearDown := testingCover.tearDown
	gotest.True(t, origTearDown != nil)

	restore := InterceptTeardown()

	// tearDown should now be the no-op
	_, err := testingCover.tearDown("", "")
	gotest.NoError(t, err)

	// Restore should bring back original
	restore()
	gotest.True(t, testingCover.tearDown != nil)
}
