package discovery_test

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// SharedFieldsTestSuite: a shared fixture field the state transfer cannot
// carry is refused at the field; what it can carry reaches the suite.
type SharedFieldsTestSuite struct{}

func (s *SharedFieldsTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second}
}

func (s *SharedFieldsTestSuite) TestEmbeddedFields(t *gotest.T) {
	t.When("a shared fixture embeds a type", func(w *gotest.T) {
		w.It("one with exported fields is refused at the field, told to hold it in a named field or assign it in Hydrate", func(it *gotest.T) {
			checkShape(it, shape{pkg: "sharedembed", exit: 2,
				shows: `fixture\.go:10:2: shared fixture "StateSharedFixture" embeds Info, whose exported fields are not transferred to the suites; hold it in a named field \(Info Info\), or assign it in Hydrate`})
		})
		w.It("one without exported fields and one Hydrate assigns are left out, and the suite runs with its state", func(it *gotest.T) {
			checkShape(it, shape{pkg: "sharedembedok", verdict: "TestStateTestSuite/TestState"})
		})
	})
}

func (s *SharedFieldsTestSuite) TestInterfaceFields(t *gotest.T) {
	t.When("a shared fixture transfers a field whose type holds an interface with methods", func(w *gotest.T) {
		w.It("the field is refused at the field, told to assign it in Hydrate or unexport it", func(it *gotest.T) {
			checkShape(it, shape{pkg: "sharediface", exit: 2,
				shows: `fixture\.go:10:2: shared fixture "ClockSharedFixture": transfer field "Clock" has non-JSON-serializable type \S*sharediface\.Clock \(interface with methods\) — assign it in Hydrate to make it a local field, or unexport it when only the setup process uses it`})
		})
		w.It("an interface field Hydrate assigns and a field of a type that refers to itself reach the suite", func(it *gotest.T) {
			checkShape(it, shape{pkg: "sharedifaceok", verdict: "TestChainTestSuite/TestState"})
		})
	})
}

func (s *SharedFieldsTestSuite) TestHydratedPromotedFields(t *gotest.T) {
	t.When("Hydrate assigns the fields a shared fixture's embedded struct promotes", func(w *gotest.T) {
		w.It("a fixture whose Hydrate assigns every one of them runs with its state", func(it *gotest.T) {
			checkShape(it, shape{pkg: "sharedembedhyd", verdict: "TestStateTestSuite/TestState"})
		})
		w.It("one whose Hydrate assigns only some is refused at the field, naming the fields it does not", func(it *gotest.T) {
			checkShape(it, shape{pkg: "sharedembedpart", exit: 2,
				shows: `fixture\.go:14:2: shared fixture "StateSharedFixture" embeds Info, whose exported fields Host and Port are neither transferred to the suites nor assigned in Hydrate; hold it in a named field \(Info Info\), or assign them in Hydrate`})
		})
	})
}
