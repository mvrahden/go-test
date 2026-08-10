package withforeign //nolint:stdlib-test

import (
	"testing"
	"time"
)

type R struct{}

func (r *R) Errorf(string, ...any) {}

func True(t *testing.T, v bool, msgAndArgs ...any)                           {}
func Equal(t *testing.T, a, b any, msgAndArgs ...any)                        {}
func Eventually(t *testing.T, waitFor, tick time.Duration, fn func(poll *R)) {}

// Lookalike names from a foreign package — no gotest rule may fire on them.
func TestForeignNames(t *testing.T) {
	a, b := 1, 2
	True(t, a == b)
	Equal(t, a, b)
	Eventually(t, time.Second, time.Millisecond, func(poll *R) {
		True(t, a == b)
	})
}
