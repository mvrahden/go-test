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

func (s *FuzzRunTestSuite) TestSplitBudget(t *gotest.T) {
	t.When("total splits evenly across targets", func(it *gotest.T) {
		it.It("divides total by n", func(it *gotest.T) {
			gotest.Equal(it, 5*time.Minute, gotestrunner.ExportSplitBudget(10*time.Minute, 2))
		})
	})

	t.When("the even split would fall below the floor", func(it *gotest.T) {
		it.It("floors at 10s", func(it *gotest.T) {
			gotest.Equal(it, 10*time.Second, gotestrunner.ExportSplitBudget(30*time.Second, 10))
		})
	})

	t.When("total is zero", func(it *gotest.T) {
		it.It("stays zero (go's default per target)", func(it *gotest.T) {
			gotest.Equal(it, time.Duration(0), gotestrunner.ExportSplitBudget(0, 5))
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
	t.It("returns 0 immediately for zero targets without needing Jobs", func(it *gotest.T) {
		code := gotestrunner.RunFuzzTargets(context.Background(), nil, gotestrunner.FuzzRunConfig{})
		gotest.Equal(it, 0, code)
	})

	t.It("defaults Jobs to max(1, GOMAXPROCS/2) when unset", func(it *gotest.T) {
		gotest.Equal(it, gotestrunner.ExportDefaultFuzzJobs(), gotestrunner.ExportResolveFuzzJobs(0))
		gotest.Equal(it, 3, gotestrunner.ExportResolveFuzzJobs(3))
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
