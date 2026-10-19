// Package slashes holds Its whose labels carry a slash, a When that groups
// two of them, and a method the X_ prefix disables.
package slashes

import "github.com/mvrahden/go-test/pkg/gotest"

type PathsTestSuite struct{}

type row struct{ Desc string }

func (s *PathsTestSuite) TestRead(t *gotest.T) {
	t.When("a path is read", func(w *gotest.T) {
		w.It("reads a/b paths", func(it *gotest.T) { gotest.True(it, true) })
		w.It("keeps snake_case/paths whole", func(it *gotest.T) { gotest.True(it, true) })
	})
	for it, r := range gotest.Each(t, []row{{"x/y"}}) {
		gotest.NotEmpty(it, r.Desc)
	}
}

func (s *PathsTestSuite) X_TestParked(t *gotest.T) {
	t.When("the method is parked", func(w *gotest.T) {
		w.It("never runs", func(it *gotest.T) { gotest.True(it, true) })
	})
}
