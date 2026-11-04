// Package declared declares methods a run of some of them leaves out, and
// one whose Each table the source cannot list; see the spec conformance
// suite.
package declared

import "github.com/mvrahden/go-test/pkg/gotest"

type DeclaredTestSuite struct{}

func (s *DeclaredTestSuite) TestRanAlone(t *gotest.T) {
	t.When("the method runs", func(w *gotest.T) {
		w.It("passes", func(it *gotest.T) {
			gotest.True(it, true)
		})
	})
}

// rows is not a literal at the Each, so only a run names its rows.
var rows = []struct{ Desc string }{{"first"}, {"second"}}

func (s *DeclaredTestSuite) TestRanRowsFromAVariable(t *gotest.T) {
	for it, r := range gotest.Each(t, rows) {
		gotest.NotEmpty(it, r.Desc)
	}
}

func (s *DeclaredTestSuite) TestLeftOut(t *gotest.T) {
	t.When("the stream leaves the method out", func(w *gotest.T) {
		w.It("has no verdict", func(it *gotest.T) {
			gotest.True(it, true)
		})
		w.It("takes no time", func(it *gotest.T) {
			gotest.True(it, true)
		})
	})
}

func (s *DeclaredTestSuite) X_TestParked(t *gotest.T) {
	t.When("the method is parked", func(w *gotest.T) {
		w.It("is skipped with it", func(it *gotest.T) {
			gotest.Fail(it, "a parked method ran")
		})
	})
}
