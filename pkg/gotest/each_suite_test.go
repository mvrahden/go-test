package gotest_test

import (
	"github.com/mvrahden/go-test/pkg/gotest"
)

// EachTestSuite tests the Each helper for parameterized subtests.
type EachTestSuite struct{}

func (s *EachTestSuite) TestEach(t *gotest.T) {
	type entry struct {
		Desc  string
		Value int
	}

	t.When("entries have Desc field", func(w *gotest.T) {
		w.It("iterates with named subtests", func(it *gotest.T) {
			var ran []string
			for sub, tc := range gotest.Each(it, []entry{
				{"first entry", 1},
				{"second entry", 2},
				{"third entry", 3},
			}) {
				ran = append(ran, tc.Desc)
				gotest.Greater(sub, tc.Value, 0)
			}
			gotest.Len(it, ran, 3)
			gotest.Equal(it, "first entry", ran[0])
		})
	})

	t.When("entries have Name field", func(w *gotest.T) {
		w.It("uses Name for subtest name", func(it *gotest.T) {
			count := 0
			for sub, tc := range gotest.Each(it, []struct {
				Name string
				OK   bool
			}{
				{"alpha", true},
				{"beta", true},
			}) {
				count++
				gotest.True(sub, tc.OK)
			}
			gotest.Equal(it, 2, count)
		})
	})

	t.When("slice is empty", func(w *gotest.T) {
		w.It("does not iterate", func(it *gotest.T) {
			count := 0
			for range gotest.Each(it, []struct{ X int }{}) {
				count++
			}
			gotest.Equal(it, 0, count)
		})
	})

	t.When("entries are typed structs", func(w *gotest.T) {
		w.It("preserves type safety", func(it *gotest.T) {
			type testCase struct {
				Name   string
				Input  int
				Expect int
			}
			for sub, tc := range gotest.Each(it, []testCase{
				{"double 2", 2, 4},
				{"double 5", 5, 10},
			}) {
				gotest.Equal(sub, tc.Expect, tc.Input*2)
			}
		})
	})

	t.When("slice is nil", func(w *gotest.T) {
		w.It("does not iterate", func(it *gotest.T) {
			count := 0
			for range gotest.Each(it, []struct{ X int }(nil)) {
				count++
			}
			gotest.Equal(it, 0, count)
		})
	})

	t.When("entries have no Desc or Name field", func(w *gotest.T) {
		w.It("uses index-based naming", func(it *gotest.T) {
			count := 0
			for sub, tc := range gotest.Each(it, []struct{ Value int }{
				{10},
				{20},
			}) {
				count++
				gotest.Greater(sub, tc.Value, 0)
			}
			gotest.Equal(it, 2, count)
		})
	})
}
