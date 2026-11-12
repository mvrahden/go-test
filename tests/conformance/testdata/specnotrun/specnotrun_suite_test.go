// Package specnotrun leaves rows of a table unrun behind a failing one and
// parks a method; see the spec conformance suite.
package specnotrun

import "github.com/mvrahden/go-test/pkg/gotest"

type NotRunTestSuite struct{}

func (s *NotRunTestSuite) TestRowsAfterAFailure(t *gotest.T) {
	for it, r := range gotest.Each(t, []struct {
		Desc string
		ok   bool
	}{{Desc: "first", ok: true}, {Desc: "second", ok: false}, {Desc: "third", ok: true}}) {
		gotest.True(it, r.ok, "the %s row fails", r.Desc)
	}
}

func (s *NotRunTestSuite) X_TestParked(t *gotest.T) {
	t.It("is parked", func(it *gotest.T) {})
}
