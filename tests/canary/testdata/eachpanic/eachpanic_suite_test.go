// Package eachpanic panics in an Each row that had not failed and had
// asserted nothing: the panic ends the suite, and the row reports no count.
package eachpanic

import "github.com/mvrahden/go-test/pkg/gotest"

type EachPanicTestSuite struct{}

type row struct {
	Desc string
	ok   bool
}

func (s *EachPanicTestSuite) TestRows(t *gotest.T) {
	for it, r := range gotest.Each(t, []row{{"first", true}, {"second", false}}) {
		if !r.ok {
			panic("canary row panic")
		}
		gotest.True(it, r.ok)
	}
}

func (s *EachPanicTestSuite) TestSibling(t *gotest.T) { gotest.True(t, true) }
