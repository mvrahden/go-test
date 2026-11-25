// Package assertcount passes leaves that assert in every place an assertion
// can be made, leaves that assert nothing, and leaves that declare how they
// assert otherwise.
package assertcount

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type CountTestSuite struct{}

func (s *CountTestSuite) TestTwo(t *gotest.T) {
	gotest.True(t, true)
	gotest.Equal(t, 1, 1)
}

func (s *CountTestSuite) TestNothing(t *gotest.T) {}

func (s *CountTestSuite) TestEverywhere(t *gotest.T) {
	gotest.True(t.Soft(), true)
	gotest.True(t.T(), true)
	gotest.True(gotest.NewT(t.T()), true)
	gotest.True(gotest.NewTWithDeadline(t, time.Minute), true)
	helper(t)
	wait := gotest.Go(t, func() { gotest.True(t, true) })
	wait()
	gotest.Eventually(t, time.Second, time.Millisecond, func(r *gotest.R) {
		gotest.True(r, true)
		gotest.True(r, true)
	})
	gotest.TimeIsNow(t, time.Now(), time.Minute)
}

func helper(t *gotest.T) { gotest.True(t, true) }

func (s *CountTestSuite) TestDeclared(t *gotest.T) {
	t.AssertsBy("the call  must\n not panic")
	_ = time.Now()
}

func (s *CountTestSuite) TestSkipped(t *gotest.T) {
	t.Skipf("not on this machine")
}

func (s *CountTestSuite) TestBehaviors(t *gotest.T) {
	t.When("a When holds Its", func(w *gotest.T) {
		w.It("asserts", func(it *gotest.T) { gotest.True(it, true) })
		w.It("asserts nothing", func(it *gotest.T) {})
		w.It("asserts on the method's T", func(it *gotest.T) { gotest.True(t, true) })
	})
}

func (s *CountTestSuite) TestRows(t *gotest.T) {
	for row, n := range gotest.Each(t, []struct {
		Desc string
		N    int
	}{{"asserting row", 1}, {"silent row", 0}}) {
		if n.N > 0 {
			gotest.True(row, true)
		}
	}
}

func (s *CountTestSuite) TestNoRows(t *gotest.T) {
	for row := range gotest.Each(t, []string{}) {
		gotest.True(row, true)
	}
}

func (s *CountTestSuite) TestDeclaredAbove(t *gotest.T) {
	t.AssertsBy("a mock controller given T()")
	t.It("is covered", func(it *gotest.T) {})
}

// HookTestSuite asserts in its hooks only.
type HookTestSuite struct{}

func (s *HookTestSuite) BeforeEach(t *gotest.T) { gotest.True(t, true) }

func (s *HookTestSuite) AfterEach(t *gotest.T) { gotest.True(t, true) }

func (s *HookTestSuite) TestBodyless(t *gotest.T) {}
