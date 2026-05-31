package gotest_test

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// Helper functions in a SEPARATE file from the test suite.
// CallerFrame resolves file:line from the call stack. Having helpers here
// means CallerFrame output says "linereport_helpers_test.go" when it
// resolves to this file, vs "linereport_suite_test.go" for the test file.
// Tests assert on the filename to verify which frame was chosen.

type testingTLike interface {
	Errorf(format string, args ...any)
	FailNow()
}

func equalHelper[V comparable](t testingTLike, expected, actual V) {
	gotest.Equal(t, expected, actual)
}

func nestedHelper[V comparable](t testingTLike, expected, actual V) {
	equalHelper(t, expected, actual)
}

func eventuallyHelper(t testingTLike) {
	gotest.Eventually(t, 50*time.Millisecond, 10*time.Millisecond, func(poll *gotest.R) {
		gotest.True(poll, false, "condition not met")
	})
}
