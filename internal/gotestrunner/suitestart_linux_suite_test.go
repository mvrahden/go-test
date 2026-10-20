//go:build linux

package gotestrunner_test

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SuiteStartTestSuite covers a suite process whose executable another
// process still holds open for writing, as a go command does while it
// writes a tool such as test2json into the shared build cache.
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
