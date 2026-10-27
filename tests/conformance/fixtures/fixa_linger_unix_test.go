//go:build unix

package fixtures_test

import (
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

func (s *FixtureLastUseTestSuite) TestReleasedOnceTheSuitesProcessesAreGone(t *gotest.T) {
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/fixalinger/")
	gotest.NoError(t, err)
	t.When("the last suite reading a shared fixture returns while a process it started still runs", func(w *gotest.T) {
		w.It("releases the fixture only once that process is gone", func(it *gotest.T) {
			gotest.True(it, run.MarkerOrder("suite returns", "a.AfterAll"), "markers: %v\nstderr: %s", run.Markers, run.Stderr)
			gotest.NotContains(it, run.Markers, "a.AfterAll with the child running")
		})
	})
}
