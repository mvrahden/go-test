package gotest_test

import (
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// EachRowsTestSuite: how a row that leaves early ends the method around it.
type EachRowsTestSuite struct{}

type eachRow struct{ Desc string }

func (s *EachRowsTestSuite) TestSkipfInARow(t *gotest.T) {
	var skipped, failed bool
	var rowsRun []string
	t.T().Run("method", func(mt *testing.T) { //nolint:suite-lifecycle // the method under test is a subtest of this one
		defer func() { skipped, failed = mt.Skipped(), mt.Failed() }()
		for it, r := range gotest.Each(gotest.NewT(mt), []eachRow{{"first"}, {"second"}, {"third"}}) {
			rowsRun = append(rowsRun, r.Desc)
			if r.Desc == "second" {
				it.Skipf("the %s row is not for this run", r.Desc)
			}
		}
	})
	t.When("a row of an Each table skips", func(w *gotest.T) {
		w.It("the method is skipped from that row, not failed", func(it *gotest.T) {
			gotest.True(it, skipped)
			gotest.False(it, failed)
			gotest.Equal(it, []string{"first", "second"}, rowsRun)
		})
	})
}
