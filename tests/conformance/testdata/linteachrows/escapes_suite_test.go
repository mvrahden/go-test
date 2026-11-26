package linteachrows

import (
	"errors"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type EscapesTestSuite struct{}

func (s *EscapesTestSuite) TestRowEscape(t *gotest.T) {
	for row, e := range gotest.Each(t, []entry{{Desc: "a"}}) {
		gotest.NoError(row.T(), e.err)
		r := row
		gotest.NoError(r.T(), e.err)
		gotest.NoError(t.T(), e.err)
	}
}

func (s *EscapesTestSuite) TestRowPairs(t *gotest.T) {
	for row, e := range gotest.Each(t, []entry{{Desc: "a", err: errors.New("boom")}}) {
		gotest.Error(t, e.err)
		gotest.ErrorContains(row, e.err, "boom")
		gotest.Error(row, e.err)
		gotest.ErrorContains(t, e.err, "boom")
		gotest.Error(row, e.err)
		gotest.ErrorContains(row, e.err, "boom")
	}
}
