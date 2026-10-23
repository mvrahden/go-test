package lifecycle_test

import (
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SuiteHooksTestSuite: a suite hook, a fixture's per-method hook or the body
// that fails, panics or overruns its Timeout; which later hooks run, what the
// verdicts are and where they are located.
type SuiteHooksTestSuite struct{}

func (s *SuiteHooksTestSuite) SuiteConfig() gotest.SuiteConfig {
	// No deadline: this suite compiles code, so its time is not a verdict.
	return gotest.SuiteConfig{Parallel: true, Timeout: gotest.NoDeadline}
}

func (s *SuiteHooksTestSuite) TestAHookOrBodyGoesWrong(t *gotest.T) {
	// Each cell names the hook and how it goes wrong; the row pins which hooks
	// run after it (ran), the verdicts, the exit code and, for an overrun or
	// a panic, where it is reported (said). A failed assertion or an overrun
	// fails the method and the suite goes on; a panic ends the suite after the
	// method's own teardown and AfterAll, as under go test.
	t.When("a suite hook or the body fails an assertion, panics or overruns the Timeout", func(w *gotest.T) {
		w.It("fails the run with exit 1 and the method or suite whose hook went wrong, located at that hook when it panics or overruns, running the later hooks the row lists and, after a panic in a method's hooks or body, no further method", func(it *gotest.T) {
			const file = "hooks_suite_test.go"
			for sub, row := range gotest.Each(it, []gridRow{
				{Desc: "suite BeforeAll fails an assertion", run: startRow(it, "suite.BeforeAll", "assert"),
					ran: []string{"fixture.BeforeAll", "suite.BeforeAll", "suite.AfterAll", "fixture.AfterAll"}, verdicts: map[string]string{suite: "fail"}, exit: 1},
				{Desc: "suite BeforeAll panics", run: startRow(it, "suite.BeforeAll", "panic"),
					ran: []string{"fixture.BeforeAll", "suite.BeforeAll", "suite.AfterAll", "fixture.AfterAll"}, verdicts: map[string]string{suite: "fail"}, exit: 1,
					said: panicAt(it, file, `panic(hook + " panicked")`, "suite.BeforeAll panicked")},
				{Desc: "suite BeforeEach fails an assertion", run: startRow(it, "suite.BeforeEach", "assert"),
					ran: bodySkipped, verdicts: map[string]string{body: "fail", next: "pass"}, exit: 1},
				{Desc: "suite BeforeEach panics", run: startRow(it, "suite.BeforeEach", "panic"),
					ran: suiteEndsBeforeBody, verdicts: map[string]string{body: "fail", next: "", suite: "fail"}, exit: 1,
					said: panicAt(it, file, `panic(hook + " panicked")`, "suite.BeforeEach panicked")},
				{Desc: "suite BeforeEach overruns the Timeout", run: startRow(it, "suite.BeforeEach", "overrun"),
					ran: bodySkipped, verdicts: map[string]string{body: "fail", next: "pass"}, exit: 1,
					said: overrunAt(it, file, `suiteHook(t, "suite.BeforeEach")`, "BeforeEach ")},
				{Desc: "suite BeforeEach waits on its context past the Timeout", run: startRow(it, "suite.BeforeEach", "wait"),
					ran: withMarkerAfter(bodySkipped, "suite.BeforeEach", "suite.BeforeEach saw its context end"), verdicts: map[string]string{body: "fail", next: "pass"}, exit: 1,
					said: overrunAt(it, file, `suiteHook(t, "suite.BeforeEach")`, "BeforeEach ")},
				{Desc: "the body fails an assertion", run: startRow(it, "body", "assert"),
					ran: cleanRun, verdicts: map[string]string{body: "fail", next: "pass"}, exit: 1},
				{Desc: "the body panics", run: startRow(it, "body", "panic"),
					ran: suiteEndsAfterBody, verdicts: map[string]string{body: "fail", next: "", suite: "fail"}, exit: 1,
					said: panicAt(it, file, `panic(hook + " panicked")`, "body panicked")},
				{Desc: "the body overruns the Timeout", run: startRow(it, "body", "overrun"),
					ran: cleanRun, verdicts: map[string]string{body: "fail", next: "pass"}, exit: 1,
					said: overrunAt(it, file, `suiteHook(t, "body")`, "")},
				{Desc: "suite AfterEach fails an assertion", run: startRow(it, "suite.AfterEach", "assert"),
					ran: cleanRun, verdicts: map[string]string{body: "fail", next: "pass"}, exit: 1},
				{Desc: "suite AfterEach panics", run: startRow(it, "suite.AfterEach", "panic"),
					ran: suiteEndsAfterBody, verdicts: map[string]string{body: "fail", next: "", suite: "fail"}, exit: 1,
					said: panicAt(it, file, `panic(hook + " panicked")`, "suite.AfterEach panicked")},
				{Desc: "suite AfterEach overruns the Timeout", run: startRow(it, "suite.AfterEach", "overrun"),
					ran: cleanRun, verdicts: map[string]string{body: "fail", next: "pass"}, exit: 1,
					said: overrunAt(it, file, `suiteHook(t, "suite.AfterEach")`, "AfterEach ")},
				{Desc: "suite AfterAll fails an assertion", run: startRow(it, "suite.AfterAll", "assert"),
					ran: cleanRun, verdicts: map[string]string{suite: "fail", body: "pass", next: "pass"}, exit: 1},
				{Desc: "suite AfterAll panics", run: startRow(it, "suite.AfterAll", "panic"),
					ran: cleanRun, verdicts: map[string]string{suite: "fail", body: "pass", next: "pass"}, exit: 1,
					said: panicAt(it, file, `panic(hook + " panicked")`, "suite.AfterAll panicked")},
			}) {
				checkGrid(sub, &row)
			}
		})
	})
}

func (s *SuiteHooksTestSuite) TestAFixtureEachHookGoesWrong(t *gotest.T) {
	// As above for a package fixture's BeforeEach and AfterEach. A fixture's
	// AfterEach pairs with a BeforeEach that returned: after one that failed
	// or panicked no hook of that method runs, after one that overran only the
	// AfterEach hooks.
	t.When("a fixture's BeforeEach or AfterEach returns an error, panics or overruns the Timeout", func(w *gotest.T) {
		w.It("fails the run with exit 1 and the method, naming the hook and located at it, running the later hooks the row lists and, after a panic, no further method", func(it *gotest.T) {
			const file = "fixture.go"
			for sub, row := range gotest.Each(it, []gridRow{
				{Desc: "fixture BeforeEach returns an error and its AfterEach does not run", run: startRow(it, "fixture.BeforeEach", "error"),
					ran: methodNeverOpened, verdicts: map[string]string{body: "fail", next: "pass"}, exit: 1,
					said: failedAt(it, file, `func (f *GridFixture) BeforeEach`, "GridFixture.BeforeEach failed: fixture.BeforeEach failed")},
				{Desc: "fixture BeforeEach panics", run: startRow(it, "fixture.BeforeEach", "panic"),
					ran: suiteEndsAtBeforeEach, verdicts: map[string]string{body: "fail", next: "", suite: "fail"}, exit: 1,
					said: panicAt(it, file, `panic(hook + " panicked")`, "fixture.BeforeEach panicked")},
				{Desc: "fixture BeforeEach overruns the Timeout", run: startRow(it, "fixture.BeforeEach", "overrun"),
					ran: suiteHooksSkipped, verdicts: map[string]string{body: "fail", next: "pass"}, exit: 1,
					said: overrunAt(it, file, `fixtureHook(ctx, "fixture.BeforeEach")`, "GridFixture.BeforeEach ")},
				{Desc: "fixture BeforeEach waits on its context past the Timeout", run: startRow(it, "fixture.BeforeEach", "wait"),
					ran: withMarkerAfter(suiteHooksSkipped, "fixture.BeforeEach", "fixture.BeforeEach saw its context end"), verdicts: map[string]string{body: "fail", next: "pass"}, exit: 1,
					said: overrunAt(it, file, `fixtureHook(ctx, "fixture.BeforeEach")`, "GridFixture.BeforeEach ")},
				{Desc: "fixture AfterEach returns an error", run: startRow(it, "fixture.AfterEach", "error"),
					ran: cleanRun, verdicts: map[string]string{body: "fail", next: "pass"}, exit: 1,
					said: failedAt(it, file, `func (f *GridFixture) AfterEach`, "GridFixture.AfterEach failed: fixture.AfterEach failed")},
				{Desc: "fixture AfterEach panics", run: startRow(it, "fixture.AfterEach", "panic"),
					ran: suiteEndsAfterBody, verdicts: map[string]string{body: "fail", next: "", suite: "fail"}, exit: 1,
					said: panicAt(it, file, `panic(hook + " panicked")`, "fixture.AfterEach panicked")},
				{Desc: "fixture AfterEach overruns the Timeout", run: startRow(it, "fixture.AfterEach", "overrun"),
					ran: cleanRun, verdicts: map[string]string{body: "fail", next: "pass"}, exit: 1,
					said: overrunAt(it, file, `fixtureHook(ctx, "fixture.AfterEach")`, "GridFixture.AfterEach ")},
			}) {
				checkGrid(sub, &row)
			}
		})
	})
}
