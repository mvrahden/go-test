// Package assertcount passes leaves whose assertion counts the canary reads
// from the raw stream: one per call wherever it is made, none for a poll's
// R, none reported by an interior or a skipped test, and a declared means.
// A test that ran a subtest through t.T().Run is no vacuous leaf.
package assertcount

import (
	"testing"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type CountTestSuite struct{}

func (s *CountTestSuite) TestEverywhere(t *gotest.T) {
	gotest.True(t.Soft(), true)
	gotest.True(t.T(), true)
	gotest.True(gotest.NewTWithDeadline(t, time.Minute), true)
	wait := gotest.Go(t, func() { gotest.True(t, true) })
	wait()
	gotest.Eventually(t, time.Second, time.Millisecond, func(r *gotest.R) { gotest.True(r, true) })
	gotest.TimeIsNow(t, time.Now(), time.Minute)
}

func (s *CountTestSuite) TestNothing(t *gotest.T) {}

func (s *CountTestSuite) TestSkipped(t *gotest.T) { t.Skipf("not here") }

func (s *CountTestSuite) TestNested(t *gotest.T) {
	t.It("asserts on the method's T", func(it *gotest.T) { gotest.True(t, true) })
	for row := range gotest.Each(t, []int{1}) {
		gotest.True(row, true)
	}
}

func (s *CountTestSuite) TestRaw(t *gotest.T) {
	t.T().Run("sub", func(tt *testing.T) {})
}

func (s *CountTestSuite) TestNoRows(t *gotest.T) {
	for row := range gotest.Each(t, []int{}) {
		gotest.True(row, true)
	}
}

func (s *CountTestSuite) TestDeclared(t *gotest.T) { t.AssertsBy("a raw   check") }

func (s *CountTestSuite) TestDeclaredAbove(t *gotest.T) {
	t.AssertsBy("a mock controller")
	t.It("is covered", func(it *gotest.T) {})
}

// HookTestSuite asserts in its hooks only.
type HookTestSuite struct{}

func (s *HookTestSuite) BeforeEach(t *gotest.T) { gotest.True(t, true) }

func (s *HookTestSuite) AfterEach(t *gotest.T) { gotest.True(t, true) }

func (s *HookTestSuite) TestBodyless(t *gotest.T) {}
