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
	// Each cell names the hook and how it goes wrong; the row pins which hooks
	// run after it (ran), the verdicts and the exit code. A failed assertion
	// or an overrun fails the method and the suite goes on; a panic ends the
	// suite after the method's own teardown and AfterAll, as under go test.
	t.When("a suite hook or the body fails an assertion, panics or overruns the Timeout", func(w *gotest.T) {
		w.It("fails the run with exit 1 and the method or suite whose hook went wrong, running the later hooks the row lists and, after a panic in a method's hooks or body, no further method", func(it *gotest.T) {
			for sub, row := range gotest.Each(it, []gridRow{
				{Desc: "suite BeforeAll fails an assertion", at: "suite.BeforeAll", mode: "assert",
					ran: []string{"fixture.BeforeAll", "suite.BeforeAll", "suite.AfterAll", "fixture.AfterAll"}, verdicts: map[string]string{suite: "fail"}, exit: 1},
				{Desc: "suite BeforeAll panics", at: "suite.BeforeAll", mode: "panic",
					ran: []string{"fixture.BeforeAll", "suite.BeforeAll", "suite.AfterAll", "fixture.AfterAll"}, verdicts: map[string]string{suite: "fail"}, exit: 1},
				{Desc: "suite BeforeEach fails an assertion", at: "suite.BeforeEach", mode: "assert",
					ran: bodySkipped, verdicts: map[string]string{body: "fail", next: "pass"}, exit: 1},
				{Desc: "suite BeforeEach panics", at: "suite.BeforeEach", mode: "panic",
					ran: suiteEndsBeforeBody, verdicts: map[string]string{body: "fail", next: "", suite: "fail"}, exit: 1},
				{Desc: "suite BeforeEach overruns the Timeout", at: "suite.BeforeEach", mode: "overrun",
					ran: bodySkipped, verdicts: map[string]string{body: "fail", next: "pass"}, exit: 1},
				{Desc: "the body fails an assertion", at: "body", mode: "assert",
					ran: cleanRun, verdicts: map[string]string{body: "fail", next: "pass"}, exit: 1},
				{Desc: "the body panics", at: "body", mode: "panic",
					ran: suiteEndsAfterBody, verdicts: map[string]string{body: "fail", next: "", suite: "fail"}, exit: 1},
				{Desc: "the body overruns the Timeout", at: "body", mode: "overrun",
					ran: cleanRun, verdicts: map[string]string{body: "fail", next: "pass"}, exit: 1},
				{Desc: "suite AfterEach fails an assertion", at: "suite.AfterEach", mode: "assert",
					ran: cleanRun, verdicts: map[string]string{body: "fail", next: "pass"}, exit: 1},
				{Desc: "suite AfterEach panics", at: "suite.AfterEach", mode: "panic",
					ran: suiteEndsAfterBody, verdicts: map[string]string{body: "fail", next: "", suite: "fail"}, exit: 1},
				{Desc: "suite AfterAll fails an assertion", at: "suite.AfterAll", mode: "assert",
					ran: cleanRun, verdicts: map[string]string{suite: "fail", body: "pass", next: "pass"}, exit: 1},
				{Desc: "suite AfterAll panics", at: "suite.AfterAll", mode: "panic",
					ran: cleanRun, verdicts: map[string]string{suite: "fail", body: "pass", next: "pass"}, exit: 1},
			}) {
				run := runGrid(sub, row.at, row.mode)
				checkGrid(sub, &row, &run)
			}
		})
	})
}
