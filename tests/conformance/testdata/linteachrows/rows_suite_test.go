package linteachrows

import (
	"errors"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type entry struct {
	Desc string
	n    *int
	err  error
}

type RowsTestSuite struct{}

func (s *RowsTestSuite) TestEndsTheMethod(t *gotest.T) {
	for row, e := range gotest.Each(t, []entry{{Desc: "a", err: errors.New("boom")}}) {
		if e.err != nil {
			row.FailNow()
		}
		if e.n == nil {
			gotest.Fail(row, "no count")
			return
		}
		r := row
		if e.err != nil {
			r.FailNow()
		}
		func() {
			if e.err != nil {
				row.FailNow()
			}
		}()
	}
}

func (s *RowsTestSuite) TestSeveralConditions(t *gotest.T) {
	for row, e := range gotest.Each(t, []entry{{Desc: "a"}}) {
		if e.n == nil || *e.n != 1 {
			gotest.Fail(row, "wrong count")
		}
		if e.n == nil {
			gotest.Fail(row, "no count")
		} else if *e.n != 1 {
			gotest.Fail(row, "wrong count")
		}
	}
}

func (s *RowsTestSuite) TestGoesOn(t *gotest.T) {
	for row, e := range gotest.Each(t, []entry{{Desc: "a"}}) {
		if e.err != nil {
			gotest.Fail(row, "unexpected error")
		}
		if e.err != nil {
			row.Errorf("unexpected: %v", e)
		}
	}
}
