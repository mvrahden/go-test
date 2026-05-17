package gotest_test

import (
	"github.com/mvrahden/go-test/pkg/gotest"
)

type EachTestSuite struct{}

func (s *EachTestSuite) TestCallbackAPI(t *gotest.T) {
	t.When("entries have Desc field", func(w *gotest.T) {
		w.It("runs each entry as a named subtest", func(it *gotest.T) {
			var ran []string
			it.Each([]struct {
				Desc  string
				Value int
			}{
				{"first entry", 1},
				{"second entry", 2},
				{"third entry", 3},
			}, func(tt *gotest.T, tc struct {
				Desc  string
				Value int
			}) {
				ran = append(ran, tc.Desc)
				gotest.Greater(tt, tc.Value, 0)
			})
			gotest.Len(it, ran, 3)
			gotest.Equal(it, "first entry", ran[0])
		})
	})

	t.When("entries have Name field", func(w *gotest.T) {
		w.It("uses Name for subtest name", func(it *gotest.T) {
			count := 0
			it.Each([]struct {
				Name string
				OK   bool
			}{
				{"alpha", true},
				{"beta", true},
			}, func(tt *gotest.T, tc struct {
				Name string
				OK   bool
			}) {
				count++
				gotest.True(tt, tc.OK)
			})
			gotest.Equal(it, 2, count)
		})
	})

	t.When("entries are anonymous structs", func(w *gotest.T) {
		w.It("runs each entry", func(it *gotest.T) {
			count := 0
			it.Each([]struct {
				Desc string
				Val  int
			}{
				{"processes first value", 10},
				{"processes second value", 20},
			}, func(tt *gotest.T, tc struct {
				Desc string
				Val  int
			}) {
				count++
				gotest.Greater(tt, tc.Val, 0)
			})
			gotest.Equal(it, 2, count)
		})
	})

	t.When("slice is empty", func(w *gotest.T) {
		w.It("runs nothing", func(it *gotest.T) {
			count := 0
			it.Each([]struct{ X int }{}, func(tt *gotest.T, tc struct{ X int }) {
				count++
			})
			gotest.Equal(it, 0, count)
		})
	})
}

func (s *EachTestSuite) TestIteratorAPI(t *gotest.T) {
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
}
