package fixtures_test

import (
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// FixtureHooksTestSuite: a package fixture hook that returns an error, panics
// or overruns the suite's Timeout; which later hooks run, the verdicts, and
// where the failure is reported.
type FixtureHooksTestSuite struct{}

func (s *FixtureHooksTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

type hookRow struct {
	Desc     string
	at, mode string
	ran      []string
	verdicts map[string]string
	exit     int
}

const (
	suite = "TestGridTestSuite"
	body  = suite + "/TestBody"
	next  = suite + "/TestNext"
)

var (
	cleanRun = []string{"fixture.BeforeAll", "suite.BeforeAll", "fixture.BeforeEach", "suite.BeforeEach", "body", "suite.AfterEach", "fixture.AfterEach", "fixture.BeforeEach", "suite.BeforeEach", "next", "suite.AfterEach", "fixture.AfterEach", "suite.AfterAll", "fixture.AfterAll"}
	// A fixture BeforeEach that failed registered no AfterEach; the suite's
	// hooks of that method never ran either.
	methodNeverOpened = []string{"fixture.BeforeAll", "suite.BeforeAll", "fixture.BeforeEach", "fixture.BeforeEach", "suite.BeforeEach", "next", "suite.AfterEach", "fixture.AfterEach", "suite.AfterAll", "fixture.AfterAll"}
	// A panic ends the suite after the method's own teardown, as under go
	// test: the next method never starts and has no verdict.
	suiteEndsAtBeforeEach = []string{"fixture.BeforeAll", "suite.BeforeAll", "fixture.BeforeEach", "suite.AfterAll", "fixture.AfterAll"}
	suiteEndsAfterBody    = []string{"fixture.BeforeAll", "suite.BeforeAll", "fixture.BeforeEach", "suite.BeforeEach", "body", "suite.AfterEach", "fixture.AfterEach", "suite.AfterAll", "fixture.AfterAll"}
	// A fixture BeforeEach that overran but returned set up what it reached:
	// the body never runs, every AfterEach releases it, and the next method runs.
	bodyNeverRan = []string{"fixture.BeforeAll", "suite.BeforeAll", "fixture.BeforeEach", "suite.BeforeEach", "suite.AfterEach", "fixture.AfterEach", "fixture.BeforeEach", "suite.BeforeEach", "next", "suite.AfterEach", "fixture.AfterEach", "suite.AfterAll", "fixture.AfterAll"}
)

func runHooks(t *gotest.T, at, mode string) testkit.Run {
	run, err := testkit.RunPackageWithEnv(t.Context(), t.TempDir(), []string{"HOOKS_AT=" + at, "HOOKS_MODE=" + mode}, "../testdata/hooks/")
	gotest.NoError(t, err)
	return run
}

func (s *FixtureHooksTestSuite) TestAFixtureHookGoesWrong(t *gotest.T) {
	// Each cell names the hook and how it goes wrong; the row pins which hooks
	// run after it (ran), the verdicts and the exit code. A fixture BeforeAll
	// that fails still gets its AfterAll; a fixture BeforeEach that fails
	// registered no AfterEach; a panic ends the suite after the method's own
	// teardown and AfterAll, as under go test.
	t.When("a package fixture hook returns an error, panics or overruns the Timeout", func(w *gotest.T) {
		w.It("fails the run, runs the later hooks its row names and gives the verdicts its row names", func(it *gotest.T) {
			for sub, row := range gotest.Each(it, []hookRow{
				{Desc: "fixture BeforeAll returns an error", at: "fixture.BeforeAll", mode: "error",
					ran: []string{"fixture.BeforeAll", "fixture.AfterAll"}, verdicts: map[string]string{suite: "fail"}, exit: 1},
				{Desc: "fixture BeforeAll panics", at: "fixture.BeforeAll", mode: "panic",
					ran: []string{"fixture.BeforeAll", "fixture.AfterAll"}, verdicts: map[string]string{suite: "fail"}, exit: 1},
				{Desc: "fixture BeforeEach returns an error", at: "fixture.BeforeEach", mode: "error",
					ran: methodNeverOpened, verdicts: map[string]string{body: "fail", next: "pass"}, exit: 1},
				{Desc: "fixture BeforeEach panics", at: "fixture.BeforeEach", mode: "panic",
					ran: suiteEndsAtBeforeEach, verdicts: map[string]string{body: "fail", next: "", suite: "fail"}, exit: 1},
				{Desc: "fixture BeforeEach overruns the Timeout", at: "fixture.BeforeEach", mode: "overrun",
					ran: bodyNeverRan, verdicts: map[string]string{body: "fail", next: "pass"}, exit: 1},
				{Desc: "fixture AfterEach returns an error", at: "fixture.AfterEach", mode: "error",
					ran: cleanRun, verdicts: map[string]string{body: "fail", next: "pass"}, exit: 1},
				{Desc: "fixture AfterEach panics", at: "fixture.AfterEach", mode: "panic",
					ran: suiteEndsAfterBody, verdicts: map[string]string{body: "fail", next: "", suite: "fail"}, exit: 1},
				{Desc: "fixture AfterAll returns an error", at: "fixture.AfterAll", mode: "error",
					ran: cleanRun, exit: 1},
				{Desc: "fixture AfterAll panics", at: "fixture.AfterAll", mode: "panic",
					ran: cleanRun, exit: 1},
			}) {
				run := runHooks(sub, row.at, row.mode)
				gotest.Equal(sub, row.exit, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
				if row.ran != nil {
					gotest.Equal(sub, row.ran, run.Markers)
				}
				for path, want := range row.verdicts {
					gotest.Equal(sub, want, run.Verdict(path), "verdict of %s", path)
				}
			}
		})
	})

	t.When("a fixture BeforeEach returns an error", func(w *gotest.T) {
		run := runHooks(w, "fixture.BeforeEach", "error")
		w.It("is reported in the hook's own file, not the runtime's", func(it *gotest.T) {
			gotest.Contains(it, run.Stdout, "fixture.go:")
			gotest.NotContains(it, run.Stdout, "methods.go:")
		})
	})

	t.When("a fixture AfterEach returns an error", func(w *gotest.T) {
		run := runHooks(w, "fixture.AfterEach", "error")
		w.It("is reported in the hook's own file, not the runtime's", func(it *gotest.T) {
			gotest.Contains(it, run.Stdout, "fixture.go:")
			gotest.NotContains(it, run.Stdout, "methods.go:")
		})
	})
}
