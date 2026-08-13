package gotestrunner_test

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"time"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// FuzzRunTestSuite tests the fuzz orchestrator's scheduling logic (budget
// splitting) and per-target `go test -fuzz` command construction.
type FuzzRunTestSuite struct{}

// TestPlanFuzzSchedule pins the --for contract: the budget is approximate
// wall-clock for the whole session, so per-target shares scale with the
// effective concurrency and waves multiply back out to ≈total.
func (s *FuzzRunTestSuite) TestPlanFuzzSchedule(t *gotest.T) {
	t.When("targets run one at a time", func(it *gotest.T) {
		plan := gotestrunner.PlanFuzzSchedule(10*time.Minute, 2, 1)

		it.It("divides total by n", func(it *gotest.T) {
			gotest.Equal(it, 5*time.Minute, plan.PerTarget)
			gotest.Equal(it, 2, plan.Waves)
			gotest.Equal(it, 10*time.Minute, plan.EstWall)
		})
	})

	t.When("jobs run several targets concurrently", func(it *gotest.T) {
		plan := gotestrunner.PlanFuzzSchedule(10*time.Minute, 10, 5)

		it.It("scales each share by the concurrency so wall-clock stays ≈total", func(it *gotest.T) {
			gotest.Equal(it, 5*time.Minute, plan.PerTarget)
			gotest.Equal(it, 2, plan.Waves)
			gotest.Equal(it, 10*time.Minute, plan.EstWall)
		})
	})

	t.When("there are fewer targets than jobs", func(it *gotest.T) {
		plan := gotestrunner.PlanFuzzSchedule(time.Minute, 2, 8)

		it.It("caps concurrency at the target count so shares never exceed total", func(it *gotest.T) {
			gotest.Equal(it, 2, plan.Jobs)
			gotest.Equal(it, time.Minute, plan.PerTarget)
			gotest.Equal(it, 1, plan.Waves)
			gotest.Equal(it, time.Minute, plan.EstWall)
		})
	})

	t.When("the share would fall below the floor", func(it *gotest.T) {
		plan := gotestrunner.PlanFuzzSchedule(30*time.Second, 10, 1)

		it.It("floors at 10s and reports the stretch", func(it *gotest.T) {
			gotest.Equal(it, 10*time.Second, plan.PerTarget)
			gotest.True(it, plan.Floored)
			gotest.Equal(it, 100*time.Second, plan.EstWall)
		})
	})

	t.When("total is zero", func(it *gotest.T) {
		plan := gotestrunner.PlanFuzzSchedule(0, 5, 2)

		it.It("leaves PerTarget zero (go's default per target)", func(it *gotest.T) {
			gotest.Equal(it, time.Duration(0), plan.PerTarget)
			gotest.False(it, plan.Floored)
			gotest.Equal(it, time.Duration(0), plan.EstWall)
		})
	})
}

func (s *FuzzRunTestSuite) TestBuildFuzzArgs(t *gotest.T) {
	target := gotestrunner.FuzzTarget{
		Package: "example.com/pkg",
		Dir:     "/abs/pkg",
		Func:    "FuzzFooTestSuite_FuzzBar",
	}
	cfg := gotestrunner.FuzzRunConfig{
		OverlayFlag: "-overlay=/tmp/overlay.json",
		BuildFlags:  []string{"-tags=integration"},
	}

	t.When("a non-zero per-target budget is given", func(it *gotest.T) {
		args := gotestrunner.ExportBuildFuzzArgs(target, cfg, 5*time.Second)

		it.It("includes the overlay flag", func(it *gotest.T) {
			gotest.Contains(it, args, "-overlay=/tmp/overlay.json")
		})
		it.It("runs no ordinary tests", func(it *gotest.T) {
			gotest.Contains(it, args, "-run=^$")
		})
		it.It("anchors and quotes the fuzz func name", func(it *gotest.T) {
			gotest.Contains(it, args, "-fuzz=^FuzzFooTestSuite_FuzzBar$")
		})
		it.It("formats the fuzztime duration", func(it *gotest.T) {
			gotest.Contains(it, args, "-fuzztime=5s")
		})
		it.It("forwards build flags", func(it *gotest.T) {
			gotest.Contains(it, args, "-tags=integration")
		})
		it.It("targets the package import path last", func(it *gotest.T) {
			gotest.Equal(it, "example.com/pkg", args[len(args)-1])
		})
	})

	t.When("the per-target budget is zero", func(it *gotest.T) {
		it.It("omits -fuzztime, deferring to go's own default", func(it *gotest.T) {
			args := gotestrunner.ExportBuildFuzzArgs(target, cfg, 0)
			for _, a := range args {
				gotest.False(it, len(a) >= len("-fuzztime=") && a[:len("-fuzztime=")] == "-fuzztime=")
			}
		})
	})
}

func (s *FuzzRunTestSuite) TestRunFuzzTargetsJobsDefault(t *gotest.T) {
	t.It("returns an empty result for zero targets without needing Jobs", func(it *gotest.T) {
		res := gotestrunner.RunFuzzTargets(context.Background(), nil, gotestrunner.FuzzRunConfig{})
		gotest.Equal(it, 0, res.ExitCode())
		gotest.Empty(it, res.Outcomes)
	})

	t.It("defaults Jobs to max(1, GOMAXPROCS/2) when unset", func(it *gotest.T) {
		gotest.Equal(it, gotestrunner.ExportDefaultFuzzJobs(), gotestrunner.ExportResolveFuzzJobs(0))
		gotest.Equal(it, 3, gotestrunner.ExportResolveFuzzJobs(3))
	})
}

// TestExitContract pins the session exit-code semantics: time exhaustion is
// the expected end of a search and never a failure by itself; findings —
// a genuine FAIL, or a new crasher file even when the shutdown killed the
// process mid-crash — are what drive a non-zero exit.
func (s *FuzzRunTestSuite) TestExitContract(t *gotest.T) {
	t.When("a target is stopped by the session ending without a finding", func(it *gotest.T) {
		o := gotestrunner.FuzzTargetOutcome{Func: "FuzzA", ExitCode: 1, Canceled: true}

		it.It("maps the shutdown-caused non-zero exit to 0", func(it *gotest.T) {
			gotest.Equal(it, 0, o.EffectiveExitCode())
		})
		it.It("counts as cut short in the session result", func(it *gotest.T) {
			res := gotestrunner.FuzzRunResult{Outcomes: []gotestrunner.FuzzTargetOutcome{o}}
			gotest.Equal(it, 0, res.ExitCode())
			gotest.Equal(it, []string{"FuzzA"}, res.CutShort())
		})
	})

	t.When("a target was killed mid-crash after writing a corpus file", func(it *gotest.T) {
		o := gotestrunner.FuzzTargetOutcome{Func: "FuzzB", ExitCode: 1, Canceled: true, NewCrashers: []string{"582528dd"}}

		it.It("still reports the finding despite the cancellation", func(it *gotest.T) {
			gotest.Equal(it, 1, o.EffectiveExitCode())
		})
	})

	t.When("a target fails genuinely while the session is still live", func(it *gotest.T) {
		it.It("keeps the subprocess exit code", func(it *gotest.T) {
			o := gotestrunner.FuzzTargetOutcome{Func: "FuzzC", ExitCode: 1}
			gotest.Equal(it, 1, o.EffectiveExitCode())
		})
		it.It("keeps a tool-level exit 2", func(it *gotest.T) {
			o := gotestrunner.FuzzTargetOutcome{Func: "FuzzC", ExitCode: 2}
			gotest.Equal(it, 2, o.EffectiveExitCode())
		})
	})

	t.When("a new crasher appears even though the process exited 0", func(it *gotest.T) {
		it.It("treats the crasher file as the authoritative finding signal", func(it *gotest.T) {
			o := gotestrunner.FuzzTargetOutcome{Func: "FuzzD", NewCrashers: []string{"deadbeef"}}
			gotest.Equal(it, 1, o.EffectiveExitCode())
		})
	})

	t.When("a target never started because the session ended first", func(it *gotest.T) {
		o := gotestrunner.FuzzTargetOutcome{Func: "FuzzE", Skipped: true}

		it.It("is not a failure", func(it *gotest.T) {
			gotest.Equal(it, 0, o.EffectiveExitCode())
		})
		it.It("is reported as cut short", func(it *gotest.T) {
			res := gotestrunner.FuzzRunResult{Outcomes: []gotestrunner.FuzzTargetOutcome{o}}
			gotest.Equal(it, []string{"FuzzE"}, res.CutShort())
		})
	})

	t.When("outcomes mix findings and cut-short targets", func(it *gotest.T) {
		res := gotestrunner.FuzzRunResult{Outcomes: []gotestrunner.FuzzTargetOutcome{
			{Func: "FuzzA", ExitCode: 1, Canceled: true},
			{Func: "FuzzB", ExitCode: 1},
			{Func: "FuzzC", Skipped: true},
		}}

		it.It("returns the worst effective code", func(it *gotest.T) {
			gotest.Equal(it, 1, res.ExitCode())
		})
		it.It("excludes the finding from the cut-short list", func(it *gotest.T) {
			gotest.Equal(it, []string{"FuzzA", "FuzzC"}, res.CutShort())
		})
	})
}

func (s *FuzzRunTestSuite) TestLineWriter(t *gotest.T) {
	t.When("a single write contains multiple newline-terminated lines", func(it *gotest.T) {
		it.It("splits and prefixes each complete line as it arrives, holding back the remainder", func(it *gotest.T) {
			var buf bytes.Buffer
			var mu sync.Mutex
			w := gotestrunner.ExportNewLineWriter(&buf, "MyFunc", &mu)

			n, err := w.Write([]byte("first\nsecond\nthird"))

			gotest.NoError(it, err)
			gotest.Len(it, "first\nsecond\nthird", n)
			gotest.Equal(it, "[MyFunc] first\n[MyFunc] second\n", buf.String())

			gotest.NoError(it, w.Close())
			gotest.Equal(it, "[MyFunc] first\n[MyFunc] second\n[MyFunc] third\n", buf.String())
		})
	})

	t.When("a line is written without a trailing newline", func(it *gotest.T) {
		it.It("flushes it as a final line on Close", func(it *gotest.T) {
			var buf bytes.Buffer
			var mu sync.Mutex
			w := gotestrunner.ExportNewLineWriter(&buf, "Trailing", &mu)

			_, err := w.Write([]byte("no newline here"))
			gotest.NoError(it, err)
			gotest.Empty(it, buf.String())

			gotest.NoError(it, w.Close())
			gotest.Equal(it, "[Trailing] no newline here\n", buf.String())
		})
	})

	t.When("a single unterminated line exceeds the buffering cap", func(it *gotest.T) {
		it.It("force-flushes it in capped, marked chunks instead of buffering without bound", func(it *gotest.T) {
			var buf bytes.Buffer
			var mu sync.Mutex
			w := gotestrunner.ExportNewLineWriter(&buf, "Huge", &mu)

			maxBuf := gotestrunner.ExportLineWriterMaxBuf
			huge := bytes.Repeat([]byte("x"), maxBuf+10)

			n, err := w.Write(huge)
			gotest.NoError(it, err)
			gotest.Len(it, huge, n)

			// One full cap-sized chunk must already have been force-flushed,
			// marked as continued, and no bytes dropped.
			gotest.Contains(it, buf.String(), "[Huge] (line continued) ")
			gotest.Equal(it, maxBuf, strings.Count(buf.String(), "x"))

			gotest.NoError(it, w.Close())
			// The rest (10 bytes) flushes on Close as the final line.
			gotest.Equal(it, maxBuf+10, strings.Count(buf.String(), "x"))
		})
	})
}
