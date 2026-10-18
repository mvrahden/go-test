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
)

func runHooks(t *gotest.T, at, mode string) testkit.Run {
	run, err := testkit.RunPackageWithEnv(t.Context(), t.TempDir(), []string{"HOOKS_AT=" + at, "HOOKS_MODE=" + mode}, "../testdata/hooks/")
	gotest.NoError(t, err)
	return run
}

func (s *FixtureHooksTestSuite) TestAFixtureHookGoesWrong(t *gotest.T) {
	t.When("a package fixture hook returns an error, panics or overruns the Timeout", func(w *gotest.T) {
		for it, row := range gotest.Each(w, []hookRow{
			{Desc: "fixture BeforeAll returns an error: its AfterAll still runs, the suite never opens and fails", at: "fixture.BeforeAll", mode: "error",
				ran: []string{"fixture.BeforeAll", "fixture.AfterAll"}, verdicts: map[string]string{suite: "fail"}, exit: 1},
			{Desc: "fixture BeforeAll panics: its AfterAll still runs, the suite never opens and fails", at: "fixture.BeforeAll", mode: "panic",
				ran: []string{"fixture.BeforeAll", "fixture.AfterAll"}, verdicts: map[string]string{suite: "fail"}, exit: 1},
			{Desc: "fixture BeforeEach returns an error: no AfterEach runs for that method, the next method runs", at: "fixture.BeforeEach", mode: "error",
				ran: methodNeverOpened, verdicts: map[string]string{body: "fail", next: "pass"}, exit: 1},
			{Desc: "fixture BeforeEach panics: no AfterEach runs for that method, and the suite ends as under go test", at: "fixture.BeforeEach", mode: "panic",
				ran: suiteEndsAtBeforeEach, verdicts: map[string]string{body: "fail", next: "", suite: "fail"}, exit: 1},
			{Desc: "fixture BeforeEach overruns the Timeout: the method fails", at: "fixture.BeforeEach", mode: "overrun",
				verdicts: map[string]string{body: "fail", next: "pass"}, exit: 1},
			{Desc: "fixture AfterEach returns an error: the method fails, the next method runs", at: "fixture.AfterEach", mode: "error",
				ran: cleanRun, verdicts: map[string]string{body: "fail", next: "pass"}, exit: 1},
			{Desc: "fixture AfterEach panics: the method fails, and the suite ends as under go test", at: "fixture.AfterEach", mode: "panic",
				ran: suiteEndsAfterBody, verdicts: map[string]string{body: "fail", next: "", suite: "fail"}, exit: 1},
			{Desc: "fixture AfterAll returns an error: the run fails", at: "fixture.AfterAll", mode: "error",
				ran: cleanRun, exit: 1},
			{Desc: "fixture AfterAll panics: the run fails", at: "fixture.AfterAll", mode: "panic",
				ran: cleanRun, exit: 1},
		}) {
			run := runHooks(it, row.at, row.mode)
			gotest.Equal(it, row.exit, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			if row.ran != nil {
				gotest.Equal(it, row.ran, run.Markers)
			}
			for path, want := range row.verdicts {
				gotest.Equal(it, want, run.Verdict(path), "verdict of %s", path)
			}
		}
	})

	t.When("a fixture BeforeEach returns an error", func(w *gotest.T) {
		run := runHooks(w, "fixture.BeforeEach", "error")
		w.It("is reported at the hook's declaration, never inside the runtime", func(it *gotest.T) {
			gotest.Contains(it, run.Stdout, "fixture.go:")
			gotest.NotContains(it, run.Stdout, "gotestruntime")
		})
	})
}
