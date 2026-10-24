//go:build linux

package gotestrunner_test

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SuiteStartTestSuite covers a suite process whose executable another
// process still holds open for writing, as a go command does while it
// writes a tool such as test2json into the shared build cache.
// Sequential: TestACancelDuringTheRetryWaitEndsTheRun and
// TestAnExecutableThatStaysBusyIsReported swap the package's suite start.
type SuiteStartTestSuite struct{}

func (s *SuiteStartTestSuite) TestAnExecutableStillBeingWrittenStartsOnceItIsDone(t *gotest.T) {
	path := filepath.Join(t.TempDir(), "pkg.test")
	gotest.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o700)) //nolint:gosec // G306: the stand-in must be executable
	writer, err := os.OpenFile(path, os.O_WRONLY, 0)                            //nolint:gosec // G304: a file in the test's temp dir
	gotest.NoError(t, err)
	released := time.AfterFunc(300*time.Millisecond, func() { _ = writer.Close() })
	defer released.Stop()
	target := gotestrunner.SuiteTarget{
		SuiteSpec:  gotestrunner.SuiteSpec{Package: "example.com/pkg", SuiteName: "TestFooSuite"},
		BinaryPath: path,
	}
	result := gotestrunner.RunSingleSuite(context.Background(), target, nil, false)
	t.When("the executable is held open for writing for a moment", func(w *gotest.T) {
		w.It("starts once the writer lets go, and runs to its end", func(it *gotest.T) {
			gotest.Equal(it, 0, result.ExitCode, "stderr: %s", result.Stderr)
		})
	})
}

func (s *SuiteStartTestSuite) TestACancelDuringTheRetryWaitEndsTheRun(t *gotest.T) {
	path := filepath.Join(t.TempDir(), "pkg.test")
	gotest.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o700)) //nolint:gosec // G306: the stand-in must be executable
	writer, err := os.OpenFile(path, os.O_WRONLY, 0)                            //nolint:gosec // G304: a file in the test's temp dir
	gotest.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	// The fourth start is followed by the longest wait, 800ms; the run is
	// cancelled 50ms into it.
	var cancelled atomic.Int64
	var starts atomic.Int32
	restoreCount := gotestrunner.ExportCountSuiteStarts(func(n int) {
		starts.Store(int32(n)) //nolint:gosec // G115: a handful of starts
		if n == 4 {
			time.AfterFunc(50*time.Millisecond, func() {
				cancelled.Store(time.Now().UnixNano())
				cancel()
			})
		}
	})
	target := gotestrunner.SuiteTarget{
		SuiteSpec:  gotestrunner.SuiteSpec{Package: "example.com/pkg", SuiteName: "TestFooSuite"},
		BinaryPath: path,
	}
	result := gotestrunner.RunSingleSuite(ctx, target, nil, false)
	returned := time.Now()
	restoreCount()
	cancel()
	gotest.NoError(t, writer.Close())
	t.When("the run is cancelled while it waits to retry a busy executable", func(w *gotest.T) {
		w.It("returns at once, without waiting out the retry", func(it *gotest.T) {
			gotest.NotZero(it, cancelled.Load())
			gotest.Less(it, returned.Sub(time.Unix(0, cancelled.Load())), 300*time.Millisecond)
		})
		w.It("starts nothing after the cancel", func(it *gotest.T) {
			gotest.Equal(it, int32(4), starts.Load())
		})
		w.It("reports the cancellation, as any cancelled start", func(it *gotest.T) {
			gotest.Equal(it, 2, result.ExitCode)
			gotest.Regexp(it, `^gotest: TestFooSuite did not start: context canceled\n$`, string(result.Stderr))
		})
	})
}

func (s *SuiteStartTestSuite) TestAnExecutableThatStaysBusyIsReported(t *gotest.T) {
	var starts atomic.Int32
	busy := &os.PathError{Op: "fork/exec", Path: "pkg.test", Err: syscall.ETXTBSY}
	restore := gotestrunner.ExportFailSuiteStarts(busy, func() { starts.Add(1) })
	// Bounded, so retries without a limit fail the test rather than hang it.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	target := gotestrunner.SuiteTarget{
		SuiteSpec:  gotestrunner.SuiteSpec{Package: "example.com/pkg", SuiteName: "TestFooSuite"},
		BinaryPath: "pkg.test",
	}
	began := time.Now()
	result := gotestrunner.RunSingleSuite(ctx, target, nil, false)
	elapsed := time.Since(began)
	cancel()
	restore()
	t.When("the executable is still busy at every start", func(w *gotest.T) {
		w.It("gives up after five starts", func(it *gotest.T) {
			gotest.Equal(it, int32(5), starts.Load())
		})
		w.It("waits 100ms, 200ms, 400ms and 800ms between them", func(it *gotest.T) {
			gotest.GreaterOrEqual(it, elapsed, 1500*time.Millisecond)
			// Well short of four waits of a second each.
			gotest.Less(it, elapsed, 3*time.Second)
		})
		w.It("reports that the suite did not start, naming the busy executable", func(it *gotest.T) {
			gotest.Equal(it, 2, result.ExitCode)
			gotest.Regexp(it, `^gotest: TestFooSuite did not start: fork/exec pkg.test: text file busy\n$`, string(result.Stderr))
		})
	})
}
