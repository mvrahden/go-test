package cli_test

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLITimeoutTestSuite: a suite an expired --timeout stopped fails its package
// even when every method it ran passed, and the run names it.
type CLITimeoutTestSuite struct{}

func (s *CLITimeoutTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 45 * time.Second}
}

func (s *CLITimeoutTestSuite) TestAMethodThatReturnsWhenItsContextEnds(t *gotest.T) {
	t.When("--timeout expires while a method waits for its context to end, and the method then returns", func(w *gotest.T) {
		// The warm run builds the binary into the cache the timed runs share,
		// so the limit times the method alone.
		cache := protocol.EnvCacheDir + "=" + filepath.Join(w.TempDir(), "cache")
		warm, err := testkit.RunPackageWithEnv(w.Context(), w.TempDir(), []string{cache, "CLITIMEOUT_WARM=1"}, "../testdata/clitimeoutctx/")
		gotest.NoError(w, err)
		gotest.Equal(w, 0, warm.Exit, "stdout: %s\nstderr: %s", warm.Stdout, warm.Stderr)
		limit := "--timeout=" + testkit.Scaled(5*time.Second).String()
		stream, err := testkit.RunPackageWithEnv(w.Context(), w.TempDir(), []string{cache}, "../testdata/clitimeoutctx/", limit)
		gotest.NoError(w, err)
		text, err := testkit.RunCLIWithEnv(w.Context(), ".", []string{cache}, limit, "../testdata/clitimeoutctx/")
		gotest.NoError(w, err)
		pkgVerdict := ""
		for _, ev := range stream.Events {
			if ev.Test == "" && strings.HasSuffix(ev.Package, "/clitimeoutctx") && (ev.Action == "pass" || ev.Action == "fail") {
				pkgVerdict = ev.Action
			}
		}

		w.It("fails the package and the run with exit 1, on the text and -json pipelines", func(it *gotest.T) {
			gotest.Equal(it, 1, stream.Exit, "stdout: %s\nstderr: %s", stream.Stdout, stream.Stderr)
			gotest.Equal(it, "fail", pkgVerdict)
			gotest.Equal(it, 1, text.Exit, "stdout: %s\nstderr: %s", text.Stdout, text.Stderr)
			gotest.Regexp(it, `(?m)^FAIL\t\S+/clitimeoutctx\t`, text.Stdout)
		})
		w.It("names the suite in the run's FAIL line, on stderr and in the stream", func(it *gotest.T) {
			gotest.Regexp(it, `FAIL: global --timeout exceeded after \S+ while running: \S+/clitimeoutctx TestWaitsTestSuite`, stream.Stderr)
			gotest.Regexp(it, `"Output":"FAIL: global --timeout exceeded after \S+ while running: \S+/clitimeoutctx TestWaitsTestSuite\\n"`, stream.Stdout)
			gotest.Regexp(it, `FAIL: global --timeout exceeded after \S+ while running: \S+/clitimeoutctx TestWaitsTestSuite`, text.Stderr)
		})
	})
}
