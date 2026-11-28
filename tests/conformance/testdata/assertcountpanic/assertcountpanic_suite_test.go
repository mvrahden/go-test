// Package assertcountpanic panics in Each rows: one that had failed, which
// ends its method, and one that had asserted nothing, which ends the suite.
package assertcountpanic

import "github.com/mvrahden/go-test/pkg/gotest"

type CountPanicTestSuite struct{}

type row struct {
	Desc string
	ok   bool
}

func (s *CountPanicTestSuite) TestAFailedRow(t *gotest.T) {
	for it, r := range gotest.Each(t, []row{{"failed", false}}) {
		gotest.True(it, r.ok)
		panic("the failed row panics")
	}
}

func (s *CountPanicTestSuite) TestRows(t *gotest.T) {
	for it, r := range gotest.Each(t, []row{{"asserts", true}, {"panics", false}}) {
		if !r.ok {
			panic("the row panics before it asserts")
		}
		gotest.True(it, r.ok)
	}
}
