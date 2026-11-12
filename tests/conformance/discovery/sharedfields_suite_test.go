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
