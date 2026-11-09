//go:build unix

package lifecycle_test

import (
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CrashVerdictTestSuite: a suite process that dies under its method fails its
// package by how it ended, and the method it died in gets no verdict unless
// testing reported one.
type CrashVerdictTestSuite struct{}

func (s *CrashVerdictTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Timeout: time.Minute, Parallel: true}
}

type crashRow struct {
	Desc  string
	crash string // LCR_CRASH
}

// crashed runs lcrrelay's CrashTestSuite with its second method ending the
// way crash names.
func crashed(t *gotest.T, crash string) testkit.Run {
	run, err := testkit.RunPackageWithEnv(t.Context(), t.TempDir(), []string{"LCR_CRASH=" + crash}, "../testdata/lcrrelay/", "-run=^TestCrashTestSuite$")
	gotest.NoError(t, err)
	return run
}

func (s *CrashVerdictTestSuite) TestASuiteProcessThatCrashes(t *gotest.T) {
	t.When("a suite process ends by os.Exit(3), a panic on a goroutine or a SIGKILL while a method runs", func(w *gotest.T) {
		w.It("fails its package and exits 1, the running method left without a verdict and the verdicts reported before the end kept", func(it *gotest.T) {
			for sub, row := range gotest.Each(it, []crashRow{
				{Desc: "os.Exit(3)", crash: "exit"},
				{Desc: "a panic on a goroutine", crash: "panic"},
				{Desc: "a SIGKILL", crash: "kill"},
			}) {
				run := crashed(sub, row.crash)
				gotest.Equal(sub, 1, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
				gotest.Equal(sub, "fail", run.Verdict("-"), "stream:\n%s", run.Stdout)
				for _, path := range []string{"TestCrashTestSuite", "TestCrashTestSuite/TestCrashes", "TestCrashTestSuite/TestAfter"} {
					gotest.Empty(sub, run.Verdict(path), "verdict of %s:\n%s", path, run.Stdout)
				}
				for _, path := range []string{"TestCrashTestSuite/TestBefore/passes", "TestCrashTestSuite/TestBefore", "TestCrashTestSuite/TestCrashes/passes_before_the_end"} {
					gotest.Equal(sub, "pass", run.Verdict(path), "verdict of %s:\n%s", path, run.Stdout)
				}
				if row.crash == "kill" {
					// Named on stderr and in the stream.
					gotest.Contains(sub, run.Stderr, "gotest: suite binary terminated by signal")
					gotest.Contains(sub, run.Stdout, "gotest: suite binary terminated by signal")
				}
			}
		})
	})
}

func (s *CrashVerdictTestSuite) TestAMethodThatPanics(t *gotest.T) {
	run := crashed(t, "method")

	t.When("a method panics, ending its suite process", func(w *gotest.T) {
		w.It("fails the method, its suite and its package, every verdict testing reported kept", func(it *gotest.T) {
			gotest.Equal(it, 1, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			for _, path := range []string{"TestCrashTestSuite/TestCrashes", "TestCrashTestSuite", "-"} {
				gotest.Equal(it, "fail", run.Verdict(path), "verdict of %s:\n%s", path, run.Stdout)
			}
			for _, path := range []string{"TestCrashTestSuite/TestBefore", "TestCrashTestSuite/TestCrashes/passes_before_the_end"} {
				gotest.Equal(it, "pass", run.Verdict(path), "verdict of %s:\n%s", path, run.Stdout)
			}
			gotest.Contains(it, run.Stdout, "panic: the method panics")
		})
	})
}
