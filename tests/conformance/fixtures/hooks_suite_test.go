package fixtures_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// FixtureHooksTestSuite: a package fixture's BeforeAll or AfterAll that goes
// wrong, and where an error a fixture's per-method hook returns is reported.
type FixtureHooksTestSuite struct{}

func (s *FixtureHooksTestSuite) SuiteConfig() gotest.SuiteConfig {
	// Estimated for Windows CI: slowest method ~69s; the budget is twice that, rounded up.
	return gotest.SuiteConfig{Parallel: true, Timeout: 3 * time.Minute}
}

// runHooks runs the hooks package with the hook at told to misbehave as mode
// says, and returns the run and the test output its events carried.
func runHooks(t *gotest.T, at, mode string) (testkit.Run, string) {
	run, err := testkit.RunPackageWithEnv(t.Context(), t.TempDir(), []string{"HOOKS_AT=" + at, "HOOKS_MODE=" + mode}, "../testdata/hooks/")
	gotest.NoError(t, err)
	return run, outputOf(&run, "")
}

// outputOf is the test output run's events carried for test, or for every
// test when test is empty.
func outputOf(run *testkit.Run, test string) string {
	var out strings.Builder
	for _, ev := range run.Events {
		if test == "" || ev.Test == test {
			out.WriteString(ev.Output)
		}
	}
	return out.String()
}

// lineOf is the line of path, relative to the testdata directory, that
// contains text.
func lineOf(t *gotest.T, path, text string) string {
	src, err := os.ReadFile(filepath.Join("..", "testdata", path))
	gotest.NoError(t, err)
	for i, line := range strings.Split(string(src), "\n") {
		if strings.Contains(line, text) {
			return strconv.Itoa(i + 1)
		}
	}
	gotest.Fail(t, "%s has no line with %q", path, text)
	return ""
}

// failedAt matches msg at the start of a failure line located at file:line
// of the line of path that contains text.
func failedAt(t *gotest.T, file, path, text, msg string) string {
	return `(?m)^\s+` + regexp.QuoteMeta(file) + `:` + lineOf(t, path, text) + `: ` + regexp.QuoteMeta(msg)
}

func (s *FixtureHooksTestSuite) TestWhereAFixtureHookErrorIsReported(t *gotest.T) {
	t.When("a fixture BeforeEach returns an error", func(w *gotest.T) {
		run, out := runHooks(w, "fixture.BeforeEach", "error")
		w.It("is reported at the hook's declaration, not in the runtime's file", func(it *gotest.T) {
			gotest.Equal(it, 1, run.Exit, "stderr: %s", run.Stderr)
			gotest.Regexp(it, failedAt(it, "fixture.go", "hooks/fixture.go", "func (f *GridFixture) BeforeEach", "GridFixture.BeforeEach failed: fixture.BeforeEach failed"), out)
			gotest.NotContains(it, out, "methods.go:")
		})
	})

	t.When("a fixture AfterEach returns an error", func(w *gotest.T) {
		run, out := runHooks(w, "fixture.AfterEach", "error")
		w.It("is reported at the hook's declaration, not in the runtime's file", func(it *gotest.T) {
			gotest.Equal(it, 1, run.Exit, "stderr: %s", run.Stderr)
			gotest.Regexp(it, failedAt(it, "fixture.go", "hooks/fixture.go", "func (f *GridFixture) AfterEach", "GridFixture.AfterEach failed: fixture.AfterEach failed"), out)
			gotest.NotContains(it, out, "methods.go:")
		})
	})

	const elsewhere = "fixturepairs/elsewhere/elsewhere.go"
	t.When("a hook another package declares returns an error, a fixture's of that package or one promoted from its type", func(w *gotest.T) {
		run, err := testkit.RunPackage(w.Context(), w.TempDir(), "../testdata/fixturepairs/", "-run=^Test(Remote|Promoted)TestSuite$")
		gotest.NoError(w, err)
		w.It("is reported at the hook's declaration, named by its file's base name", func(it *gotest.T) {
			gotest.Equal(it, 1, run.Exit, "stderr: %s", run.Stderr)
			gotest.Regexp(it, failedAt(it, "elsewhere.go", elsewhere, "func (f *RemoteFixture) BeforeEach", "elsewhere_RemoteFixture.BeforeEach failed: the remote fixture is not ready"), outputOf(&run, "TestRemoteTestSuite/TestBody"))
			gotest.Regexp(it, failedAt(it, "elsewhere.go", elsewhere, "func (h *Hooks) AfterEach", "PromotedFixture.AfterEach failed: the embedded hook did not let go"), outputOf(&run, "TestPromotedTestSuite/TestBody"))
		})
	})

	t.When("a hook another package declares returns an error under -fullpath", func(w *gotest.T) {
		run, err := testkit.RunPackage(w.Context(), w.TempDir(), "../testdata/fixturepairs/", "-run=^Test(Remote|Promoted)TestSuite$", "-fullpath")
		gotest.NoError(w, err)
		w.It("names the hook's file by its full path, as go test -fullpath does", func(it *gotest.T) {
			gotest.Equal(it, 1, run.Exit, "stderr: %s", run.Stderr)
			file := filepath.ToSlash(gotest.Must(filepath.Abs(filepath.Join("..", "testdata", elsewhere))))
			gotest.Regexp(it, failedAt(it, file, elsewhere, "func (f *RemoteFixture) BeforeEach", "elsewhere_RemoteFixture.BeforeEach failed"), outputOf(&run, "TestRemoteTestSuite/TestBody"))
			gotest.Regexp(it, failedAt(it, file, elsewhere, "func (h *Hooks) AfterEach", "PromotedFixture.AfterEach failed"), outputOf(&run, "TestPromotedTestSuite/TestBody"))
		})
	})
}

const (
	gridSuite = "TestGridTestSuite"
	gridBody  = gridSuite + "/TestBody"
	gridNext  = gridSuite + "/TestNext"
)

// gridCleanRun is every hook of the hooks package, in order, when none goes wrong.
var gridCleanRun = []string{"fixture.BeforeAll", "suite.BeforeAll", "fixture.BeforeEach", "suite.BeforeEach", "body", "suite.AfterEach", "fixture.AfterEach", "fixture.BeforeEach", "suite.BeforeEach", "next", "suite.AfterEach", "fixture.AfterEach", "suite.AfterAll", "fixture.AfterAll"}

type setupRow struct {
	Desc     string
	at, mode string
	ran      []string
	verdicts map[string]string
}

func (s *FixtureHooksTestSuite) TestAFixtureBeforeAllOrAfterAllGoesWrong(t *gotest.T) {
	// Each row names the hook and how it goes wrong, and pins which hooks run
	// (ran) and the verdicts. A BeforeAll that fails still gets its AfterAll,
	// and the suite never opens.
	t.When("a package fixture's BeforeAll or AfterAll returns an error or panics", func(w *gotest.T) {
		w.It("fails the run with exit 1, runs the hooks its row names and gives the verdicts its row names", func(it *gotest.T) {
			for sub, row := range gotest.Each(it, []setupRow{
				{Desc: "fixture BeforeAll returns an error", at: "fixture.BeforeAll", mode: "error",
					ran: []string{"fixture.BeforeAll", "fixture.AfterAll"}, verdicts: map[string]string{gridSuite: "fail", gridBody: ""}},
				{Desc: "fixture BeforeAll panics", at: "fixture.BeforeAll", mode: "panic",
					ran: []string{"fixture.BeforeAll", "fixture.AfterAll"}, verdicts: map[string]string{gridSuite: "fail", gridBody: ""}},
				{Desc: "fixture AfterAll returns an error", at: "fixture.AfterAll", mode: "error",
					ran: gridCleanRun, verdicts: map[string]string{gridBody: "pass", gridNext: "pass"}},
				{Desc: "fixture AfterAll panics", at: "fixture.AfterAll", mode: "panic",
					ran: gridCleanRun, verdicts: map[string]string{gridBody: "pass", gridNext: "pass"}},
			}) {
				run, _ := runHooks(sub, row.at, row.mode)
				gotest.Equal(sub, 1, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
				gotest.Equal(sub, row.ran, run.Markers)
				for path, want := range row.verdicts {
					gotest.Equal(sub, want, run.Verdict(path), "verdict of %s", path)
				}
			}
		})
	})
}
