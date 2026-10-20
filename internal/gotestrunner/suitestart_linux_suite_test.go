//go:build linux

package gotestrunner_test

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SuiteStartTestSuite covers a suite process whose executable another
// process still holds open for writing, as a go command does while it
// writes a tool such as test2json into the shared build cache.
// Sequential: TestACancelDuringTheRetryWaitEndsTheRun swaps the package's
// suite start.
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
