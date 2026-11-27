package discovery_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// GenerateUnderFocusTestSuite: gotest generate writes every package's
// harness whole; only a run narrows by focus.
type GenerateUnderFocusTestSuite struct{}

func (s *GenerateUnderFocusTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 60 * time.Second}
}

func (s *GenerateUnderFocusTestSuite) TestAFocusedMethodAmongThePackages(t *gotest.T) {
	dir, cli := stageGenerated(t, "focusmethod")

	var stdout, stderr bytes.Buffer
	gen := exec.CommandContext(t.Context(), cli, "generate", "./focusmethod/a", "./focusmethod/b") //nolint:gosec // G204: the repo's own CLI with fixed arguments
	gen.Dir = dir
	gen.Env = testkit.ChildEnv(t.TempDir())
	gen.Stdout, gen.Stderr = &stdout, &stderr
	genErr := gen.Run()

	harnessA, errA := os.ReadFile(filepath.Join(dir, "focusmethod", "a", "gotest_psuite_test.go")) //nolint:gosec // G304: a file in the test's own temp dir
	harnessB, errB := os.ReadFile(filepath.Join(dir, "focusmethod", "b", "gotest_psuite_test.go")) //nolint:gosec // G304: a file in the test's own temp dir

	goTest := exec.CommandContext(t.Context(), "go", "test", "-json", "-count=1", "./...")
	goTest.Dir = dir
	goTest.Env = testkit.ChildEnv(t.TempDir())
	goTestOut, _ := goTest.Output()
	verdicts := map[string]string{}
	for _, ev := range testkit.ParseEvents(goTestOut) {
		switch ev.Action {
		case "pass", "fail", "skip":
			verdicts[ev.Test] = ev.Action
		}
	}

	run, runErr := testkit.RunCLI(t.Context(), dir, "-json", "./...")

	t.When("gotest generate is given two packages of which one holds a focused method", func(w *gotest.T) {
		w.It("each package keeps a harness holding every method it declares, and go test on them runs the unfocused methods too", func(it *gotest.T) {
			gotest.NoError(it, genErr, "stderr: %s", stderr.String())
			gotest.NoError(it, errA)
			gotest.NoError(it, errB)
			gotest.Contains(it, string(harnessA), `{Name: "F_TestFocused", Run:`)
			gotest.Contains(it, string(harnessA), `{Name: "TestPlain", Run:`)
			gotest.Contains(it, string(harnessB), `{Name: "TestRuns", Run:`)
			gotest.Equal(it, "pass", verdicts["TestMethodTestSuite/F_TestFocused"], "go test output:\n%s", goTestOut)
			gotest.Equal(it, "pass", verdicts["TestMethodTestSuite/TestPlain"], "go test output:\n%s", goTestOut)
			gotest.Equal(it, "pass", verdicts["TestOtherTestSuite/TestRuns"], "go test output:\n%s", goTestOut)
		})
		w.It("generate notes on stderr that it ignores F_, naming the method where it is declared, and exits 0", func(it *gotest.T) {
			gotest.NoError(it, genErr, "stderr: %s", stderr.String())
			gotest.Regexp(it, `note: generate ignores F_: each harness holds every suite and method its package declares \(1 focused, first \S*a_suite_test\.go:8 MethodTestSuite\.F_TestFocused\) — go test runs them all, and a gotest run narrows to the focus\n`, filepath.ToSlash(stderr.String()))
			gotest.Equal(it, 2, strings.Count(stdout.String(), "\n"), "stdout: %s", stdout.String())
		})
		w.It("a run of the same packages still runs the focused method alone", func(it *gotest.T) {
			gotest.NoError(it, runErr)
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Equal(it, "pass", run.Verdict("TestMethodTestSuite/F_TestFocused"), "stdout: %s", run.Stdout)
			gotest.Empty(it, run.Verdict("TestMethodTestSuite/TestPlain"))
			gotest.Equal(it, "skip", run.Verdict("TestOtherTestSuite"), "stdout: %s", run.Stdout)
			gotest.Empty(it, run.Verdict("TestOtherTestSuite/TestRuns"))
		})
	})
}
