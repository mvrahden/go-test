package table

import "github.com/mvrahden/go-test/pkg/gotest"

// Table-driven cases via gotest.Each: one behavior per row, named from Desc.
// The function under test lives in classify.go so this package also carries
// real source for coverage runs to measure.
type TableTestSuite struct{}

func (s *TableTestSuite) TestClassify(t *gotest.T) {
	t.When("classifying a number", func(w *gotest.T) {
		for sub, tc := range gotest.Each(w, []struct {
			Desc   string
			in     int
			expect string
		}{
			{Desc: "negative", in: -1, expect: "negative"},
			{Desc: "zero", in: 0, expect: "zero"},
			{Desc: "positive", in: 1, expect: "positive"},
		}) {
			gotest.Equal(sub, tc.expect, classify(tc.in))
		}
	})
}
