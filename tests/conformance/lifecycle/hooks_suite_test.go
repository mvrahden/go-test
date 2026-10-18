package lifecycle_test

import (
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SuiteHooksTestSuite: a suite hook or body that fails, panics or overruns
// its Timeout; which later hooks run and what the verdicts are.
type SuiteHooksTestSuite struct{}

func (s *SuiteHooksTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *SuiteHooksTestSuite) TestAHookOrBodyGoesWrong(t *gotest.T) {
	t.When("a suite hook or the body fails, panics or overruns the Timeout", func(w *gotest.T) {
		for it, row := range gotest.Each(w, []gridRow{
			{Desc: "suite BeforeAll fails an assertion: AfterAll and the fixture's AfterAll still run, no method does", at: "suite.BeforeAll", mode: "assert",
				ran: []string{"fixture.BeforeAll", "suite.BeforeAll", "suite.AfterAll", "fixture.AfterAll"}, verdicts: map[string]string{suite: "fail"}, exit: 1},
			{Desc: "suite BeforeAll panics: AfterAll and the fixture's AfterAll still run, no method does", at: "suite.BeforeAll", mode: "panic",
				ran: []string{"fixture.BeforeAll", "suite.BeforeAll", "suite.AfterAll", "fixture.AfterAll"}, verdicts: map[string]string{suite: "fail"}, exit: 1},
			{Desc: "suite BeforeEach fails an assertion: the body is skipped, both AfterEach hooks run, the next method runs", at: "suite.BeforeEach", mode: "assert",
				ran: bodySkipped, verdicts: map[string]string{body: "fail", next: "pass"}, exit: 1},
			{Desc: "suite BeforeEach panics: the body is skipped, both AfterEach hooks run, and the suite ends as under go test", at: "suite.BeforeEach", mode: "panic",
				ran: suiteEndsBeforeBody, verdicts: map[string]string{body: "fail", next: "", suite: "fail"}, exit: 1},
			{Desc: "suite BeforeEach overruns the Timeout: the method fails", at: "suite.BeforeEach", mode: "overrun",
				verdicts: map[string]string{body: "fail", next: "pass"}, exit: 1},
			{Desc: "the body fails an assertion: both AfterEach hooks run, the next method runs", at: "body", mode: "assert",
				ran: cleanRun, verdicts: map[string]string{body: "fail", next: "pass"}, exit: 1},
			{Desc: "the body panics: both AfterEach hooks run, and the suite ends as under go test", at: "body", mode: "panic",
				ran: suiteEndsAfterBody, verdicts: map[string]string{body: "fail", next: "", suite: "fail"}, exit: 1},
			{Desc: "the body overruns the Timeout: the method fails, the next method runs", at: "body", mode: "overrun",
				ran: cleanRun, verdicts: map[string]string{body: "fail", next: "pass"}, exit: 1},
			{Desc: "suite AfterEach fails an assertion: the fixture's AfterEach runs, the method fails, the next method runs", at: "suite.AfterEach", mode: "assert",
				ran: cleanRun, verdicts: map[string]string{body: "fail", next: "pass"}, exit: 1},
			{Desc: "suite AfterEach panics: the fixture's AfterEach runs, the method fails, and the suite ends as under go test", at: "suite.AfterEach", mode: "panic",
				ran: suiteEndsAfterBody, verdicts: map[string]string{body: "fail", next: "", suite: "fail"}, exit: 1},
			{Desc: "suite AfterAll fails an assertion: the fixture's AfterAll runs and the suite fails", at: "suite.AfterAll", mode: "assert",
				ran: cleanRun, verdicts: map[string]string{suite: "fail", body: "pass", next: "pass"}, exit: 1},
			{Desc: "suite AfterAll panics: the fixture's AfterAll runs and the suite fails", at: "suite.AfterAll", mode: "panic",
				ran: cleanRun, verdicts: map[string]string{suite: "fail", body: "pass", next: "pass"}, exit: 1},
		}) {
			run := runGrid(it, row.at, row.mode)
			checkGrid(it, row, &run)
		}
	})
}
