package gotest_test

import (
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// EachRowsTestSuite: how a row that leaves early ends the method around it.
type EachRowsTestSuite struct{}

func (s *EachRowsTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

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

func (s *EachRowsTestSuite) TestMethodSkipsInARow(t *gotest.T) {
	var skipped, failed bool
	var rowsRun []string
	t.T().Run("method", func(mt *testing.T) { //nolint:suite-lifecycle // the method under test is a subtest of this one
		defer func() { skipped, failed = mt.Skipped(), mt.Failed() }()
		method := gotest.NewT(mt)
		for _, r := range gotest.Each(method, []eachRow{{"first"}, {"second"}}) {
			rowsRun = append(rowsRun, r.Desc)
			method.Skipf("the method is not for this run")
		}
	})
	t.When("a row skips the method's own T", func(w *gotest.T) {
		w.It("the method is skipped there, not failed", func(it *gotest.T) {
			gotest.True(it, skipped)
			gotest.False(it, failed)
			gotest.Equal(it, []string{"first"}, rowsRun)
		})
	})
}
