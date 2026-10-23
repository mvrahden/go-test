package testpkg

import (
	"context"
	"fmt"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// budget is every suite's Timeout here; fallback ends a hook that waits on
// its context should that context never end.
const (
	budget   = 300 * time.Millisecond
	fallback = 3 * time.Second
)

// waitFor blocks on ctx, printing whether it ended or the fallback did.
func waitFor(ctx context.Context, who string) {
	select {
	case <-ctx.Done():
		fmt.Printf("MARK:%s saw %v, cause %v\n", who, ctx.Err(), context.Cause(ctx))
	case <-time.After(fallback):
		fmt.Printf("MARK:%s fell back\n", who)
	}
}

type WaitingFixture struct{}

func (f *WaitingFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *WaitingFixture) BeforeEach(ctx context.Context) error {
	derived, cancel := context.WithCancel(ctx)
	defer cancel()
	waitFor(derived, "fixture BeforeEach") // SITE:fixture-before
	return nil
}
func (f *WaitingFixture) AfterEach(ctx context.Context) error {
	fmt.Printf("MARK:fixture AfterEach ctx %v\n", ctx.Err())
	return nil
}

// FixtureWaitsTestSuite: the fixture's BeforeEach waits on its context.
type FixtureWaitsTestSuite struct{ Waiting *WaitingFixture }

func (s *FixtureWaitsTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Timeout: budget}
}
func (s *FixtureWaitsTestSuite) BeforeEach(t *gotest.T) { fmt.Println("MARK:suite BeforeEach") }
func (s *FixtureWaitsTestSuite) AfterEach(t *gotest.T)  { fmt.Println("MARK:suite AfterEach") }
func (s *FixtureWaitsTestSuite) TestOne(t *gotest.T)    { fmt.Println("MARK:body") }

// BeforeEachWaitsTestSuite: the suite's BeforeEach waits on its context.
type BeforeEachWaitsTestSuite struct{}

func (s *BeforeEachWaitsTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Timeout: budget}
}
func (s *BeforeEachWaitsTestSuite) BeforeEach(t *gotest.T) {
	waitFor(t.Context(), "suite BeforeEach") // SITE:suite-before
}
func (s *BeforeEachWaitsTestSuite) AfterEach(t *gotest.T) { fmt.Println("MARK:suite AfterEach") }
func (s *BeforeEachWaitsTestSuite) TestOne(t *gotest.T)   { fmt.Println("MARK:body") }

// AfterEachSleepsTestSuite: the suite's AfterEach ignores its context.
type AfterEachSleepsTestSuite struct{}

func (s *AfterEachSleepsTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Timeout: budget}
}
func (s *AfterEachSleepsTestSuite) AfterEach(t *gotest.T) {
	time.Sleep(time.Second) // SITE:suite-after
}
func (s *AfterEachSleepsTestSuite) TestOne(t *gotest.T) {}

type SleepingFixture struct{}

func (f *SleepingFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *SleepingFixture) AfterEach(ctx context.Context) error {
	time.Sleep(time.Second) // SITE:fixture-after
	return nil
}

// FixtureAfterEachSleepsTestSuite: the fixture's AfterEach ignores its context.
type FixtureAfterEachSleepsTestSuite struct{ Sleeping *SleepingFixture }

func (s *FixtureAfterEachSleepsTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Timeout: budget}
}
func (s *FixtureAfterEachSleepsTestSuite) TestOne(t *gotest.T) {}

// BodySleepsTestSuite: the body ignores its context.
type BodySleepsTestSuite struct{}

func (s *BodySleepsTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Timeout: budget}
}
func (s *BodySleepsTestSuite) TestOne(t *gotest.T) {
	time.Sleep(time.Second) // SITE:body
}

// FailFastTestSuite fails its first method.
type FailFastTestSuite struct{}

func (s *FailFastTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{FailFast: true}
}
func (s *FailFastTestSuite) TestA(t *gotest.T) { gotest.Fail(t, "first") }
func (s *FailFastTestSuite) TestB(t *gotest.T) {}

// ParallelFailFastTestSuite: each method fails only in its cleanup, on the
// wait of a goroutine that failed; whichever runs first must stop the other.
type ParallelFailFastTestSuite struct{}

func (s *ParallelFailFastTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{FailFast: true, Parallel: true}
}

// failsInCleanup starts a goroutine that fails only once its wait, a cleanup
// of the method, has begun: the cleanup registered after it releases it.
func failsInCleanup(t *gotest.T, name string) {
	fmt.Println("MARK:ran " + name)
	release := make(chan struct{})
	gotest.Go(t, func() {
		<-release
		gotest.Fail(t, "the goroutine failed")
	})
	t.T().Cleanup(func() { close(release) })
}

func (s *ParallelFailFastTestSuite) TestA(t *gotest.T) { failsInCleanup(t, "TestA") }
func (s *ParallelFailFastTestSuite) TestB(t *gotest.T) { failsInCleanup(t, "TestB") }
