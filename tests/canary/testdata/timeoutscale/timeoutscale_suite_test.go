// Package timeoutscale has a suite with a 1s Timeout whose methods outlast
// it by half and by more than double, and suites bound to fixtures with a
// 1s Timeout whose BeforeAll outlasts it by half and by more than double.
// None of them honours its context, so only the budget decides the verdict.
// One method reads the environment a process it started would inherit.
package timeoutscale

import (
	"context"
	"os"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type BudgetedTestSuite struct{}

func (s *BudgetedTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Timeout: time.Second, Parallel: true}
}

func (s *BudgetedTestSuite) TestRunsHalfPastItsTimeout(t *gotest.T) {
	time.Sleep(1500 * time.Millisecond)
}

func (s *BudgetedTestSuite) TestRunsPastTwiceItsTimeout(t *gotest.T) {
	time.Sleep(2500 * time.Millisecond)
}

func (s *BudgetedTestSuite) TestStartsAProcess(t *gotest.T) {
	gotest.Empty(t, os.Getenv("GOTEST_TIMEOUT_SCALE"), "the environment a started process inherits")
}

type SlowFixture struct{}

func (f *SlowFixture) FixtureConfig() gotest.FixtureConfig {
	return gotest.FixtureConfig{Timeout: time.Second}
}

func (f *SlowFixture) BeforeAll(ctx context.Context) error {
	time.Sleep(1500 * time.Millisecond)
	return nil
}

type FixtureBoundTestSuite struct {
	Slow *SlowFixture
}

func (s *FixtureBoundTestSuite) TestRuns(t *gotest.T) {}

type SlowerFixture struct{}

func (f *SlowerFixture) FixtureConfig() gotest.FixtureConfig {
	return gotest.FixtureConfig{Timeout: time.Second}
}

func (f *SlowerFixture) BeforeAll(ctx context.Context) error {
	time.Sleep(2500 * time.Millisecond)
	return nil
}

type SlowerFixtureBoundTestSuite struct {
	Slower *SlowerFixture
}

func (s *SlowerFixtureBoundTestSuite) TestRuns(t *gotest.T) {}
