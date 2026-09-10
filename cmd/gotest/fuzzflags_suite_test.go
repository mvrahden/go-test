package main_test

import (
	"time"

	main "github.com/mvrahden/go-test/cmd/gotest"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// FuzzFlagsTestSuite pins the parsing of the fuzz session's own flags.
type FuzzFlagsTestSuite struct{}

func (s *FuzzFlagsTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *FuzzFlagsTestSuite) TestForFlag(t *gotest.T) {
	t.It("is absent when not given, so the default budget applies", func(it *gotest.T) {
		d, explicit, err := main.ExportParseForFlag([]string{"--jobs=2"})
		gotest.NoError(it, err)
		gotest.False(it, explicit)
		gotest.Equal(it, time.Duration(0), d)
	})
	t.It("parses a Go duration", func(it *gotest.T) {
		d, explicit, err := main.ExportParseForFlag([]string{"--for=90s"})
		gotest.NoError(it, err)
		gotest.True(it, explicit)
		gotest.Equal(it, 90*time.Second, d)
	})
	t.It("takes 0 as the explicit opt-out, like --timeout=0", func(it *gotest.T) {
		d, explicit, err := main.ExportParseForFlag([]string{"--for=0"})
		gotest.NoError(it, err)
		gotest.True(it, explicit)
		gotest.Equal(it, time.Duration(0), d)
	})
	for sub, raw := range gotest.Each(t, []string{"--for=soon", "--for=-1m"}) {
		sub.It("rejects "+raw, func(it *gotest.T) {
			_, _, err := main.ExportParseForFlag([]string{raw})
			gotest.ErrorContains(it, err, "invalid --for value")
		})
	}
}

// TestSessionPlan pins the one-clock model: --for is the session's only
// clock and the deadline follows it.
func (s *FuzzFlagsTestSuite) TestSessionPlan(t *gotest.T) {
	t.When("nothing is given", func(w *gotest.T) {
		got := main.ExportPlanFuzzSession(false, 0, 1, 1)
		w.It("budgets one minute", func(it *gotest.T) {
			gotest.Equal(it, time.Minute, got.Budget)
			gotest.True(it, got.DefaultedBudget)
		})
		w.It("derives the deadline from the schedule plus headroom", func(it *gotest.T) {
			gotest.Equal(it, 3*time.Minute, got.Deadline)
		})
	})

	t.It("lets a long --for carry its own deadline", func(it *gotest.T) {
		got := main.ExportPlanFuzzSession(true, 20*time.Minute, 1, 1)
		gotest.Equal(it, 20*time.Minute, got.Budget)
		gotest.Equal(it, 30*time.Minute, got.Deadline)
	})

	t.It("counts the waves a target count needs into the deadline", func(it *gotest.T) {
		// Five targets one at a time: 12s each, 60s of waves, plus headroom.
		got := main.ExportPlanFuzzSession(true, time.Minute, 5, 1)
		gotest.Equal(it, 12*time.Second, got.Plan.PerTarget)
		gotest.Equal(it, 3*time.Minute, got.Deadline)
	})

	t.It("runs until interrupted with no deadline under --for=0", func(it *gotest.T) {
		got := main.ExportPlanFuzzSession(true, 0, 3, 1)
		gotest.Equal(it, time.Duration(0), got.Budget)
		gotest.Equal(it, time.Duration(0), got.Deadline)
	})
}
