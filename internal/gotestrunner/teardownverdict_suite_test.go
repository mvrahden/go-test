package gotestrunner_test

import (
	"errors"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// TeardownVerdictTestSuite: what the shared fixture teardown books as the
// run's failure, from its error and the fixtures' reports.
type TeardownVerdictTestSuite struct{}

func (s *TeardownVerdictTestSuite) TestAnAfterAllReportIsAFailure(t *gotest.T) {
	setup := "DownSharedFixture.BeforeAll failed after 1 attempt(s): the fixture would not come up"
	teardown := "ReleaseSharedFixture.AfterAll failed: the fixture would not release what it holds"

	t.When("the teardown returned no error", func(w *gotest.T) {
		// A sibling's setup failure claimed the process's exit status: the
		// AfterAll that failed beside it is known from its report alone, as
		// when the run stopped before a release could say so.
		w.It("fails with the AfterAll reports", func(it *gotest.T) {
			reports, err := gotestrunner.ExportTeardownVerdict(nil, []string{setup, teardown})
			gotest.Error(it, err)
			gotest.Equal(it, []string{teardown}, reports)
		})
		w.It("passes when no AfterAll failed", func(it *gotest.T) {
			_, err := gotestrunner.ExportTeardownVerdict(nil, []string{setup})
			gotest.NoError(it, err)
		})
	})

	t.When("the teardown returned an error", func(w *gotest.T) {
		w.It("keeps it and attaches the AfterAll reports", func(it *gotest.T) {
			released := errors.New("early shared fixture teardown: ReleaseSharedFixture: teardown failed")
			reports, err := gotestrunner.ExportTeardownVerdict(released, []string{setup, teardown})
			gotest.ErrorIs(it, err, released)
			gotest.Equal(it, []string{teardown}, reports)
		})
	})
}
