// Package eachskip runs Each tables whose rows skip, and whose rows end the
// method's own T.
package eachskip

import (
	"github.com/mvrahden/go-test/pkg/gotest"
)

// EachSkipTestSuite: each method ends in a row.
type EachSkipTestSuite struct{}

type row struct{ Desc string }

func (s *EachSkipTestSuite) TestASkippingRow(t *gotest.T) {
	for it, r := range gotest.Each(t, []row{{"first"}, {"second"}, {"third"}}) {
		if r.Desc == "second" {
			it.Skipf("the %s row is not for this run", r.Desc)
		}
	}
}

func (s *EachSkipTestSuite) TestMethodSkipsInARow(t *gotest.T) {
	for range gotest.Each(t, []row{{"first"}, {"second"}}) {
		t.Skipf("the method is not for this run")
	}
}

func (s *EachSkipTestSuite) TestMethodFailsInARow(t *gotest.T) {
	for range gotest.Each(t, []row{{"first"}, {"second"}}) {
		gotest.Fail(t, "the method fails in a row")
	}
}
