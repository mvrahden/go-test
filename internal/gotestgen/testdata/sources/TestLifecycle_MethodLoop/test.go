package testpkg

import (
	"context"
	"errors"
	"fmt"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type HookFixture struct{}

func (f *HookFixture) BeforeAll(ctx context.Context) error  { return nil }
func (f *HookFixture) BeforeEach(ctx context.Context) error { fmt.Println("MARK:fixture before"); return nil }
func (f *HookFixture) AfterEach(ctx context.Context) error  { fmt.Println("MARK:fixture after"); return nil }

// OrderTestSuite records the per-method order: fixture hooks wrap the suite's.
type OrderTestSuite struct{ Hook *HookFixture }

func (s *OrderTestSuite) BeforeEach(t *gotest.T) { fmt.Println("MARK:suite before") }
func (s *OrderTestSuite) AfterEach(t *gotest.T)  { fmt.Println("MARK:suite after") }
func (s *OrderTestSuite) TestOne(t *gotest.T)    { fmt.Println("MARK:method") }

type BrokenHookFixture struct{ Hook *HookFixture }

func (f *BrokenHookFixture) BeforeAll(ctx context.Context) error  { return nil }
func (f *BrokenHookFixture) BeforeEach(ctx context.Context) error { return errors.New("boom") }
func (f *BrokenHookFixture) AfterEach(ctx context.Context) error {
	fmt.Println("MARK:broken after")
	return nil
}

// BrokenHookTestSuite never reaches its method: the fixture's BeforeEach fails.
type BrokenHookTestSuite struct{ Broken *BrokenHookFixture }

func (s *BrokenHookTestSuite) TestNever(t *gotest.T) { fmt.Println("MARK:never") }

// FailFastTestSuite stops at its first failure.
type FailFastTestSuite struct{}

func (s *FailFastTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{FailFast: true}
}

func (s *FailFastTestSuite) TestA(t *gotest.T) { gotest.Fail(t, "first") }
func (s *FailFastTestSuite) TestB(t *gotest.T) { fmt.Println("MARK:after failfast") }

type loopCtx struct{ n int }

// SiteTestSuite fails an assertion through a helper in each method shape that
// does not take the plain path: the report must name the method's line too.
type SiteTestSuite struct{}

func (s *SiteTestSuite) BeforeEach(t *gotest.T) *loopCtx { return &loopCtx{n: 1} }

func (s *SiteTestSuite) TestReturning(t *gotest.T, c *loopCtx) {
	check(t, 2, c.n) // SITE:returning
}

func check(t *gotest.T, want, got int) { gotest.Equal(t, want, got) }

// ServerTestSuite starts a goroutine in BeforeAll that only AfterAll stops;
// the wait on it must come after AfterAll.
type ServerTestSuite struct{ stop chan struct{} }

func (s *ServerTestSuite) BeforeAll(t *gotest.T) {
	s.stop = make(chan struct{})
	gotest.Go(t, func() { <-s.stop })
}

func (s *ServerTestSuite) AfterAll(t *gotest.T) {
	close(s.stop)
	fmt.Println("MARK:server stopped")
}

func (s *ServerTestSuite) TestServed(t *gotest.T) { fmt.Println("MARK:served") }

func (s *ServerTestSuite) BenchmarkServed(b *gotest.B) {
	for b.Loop() {
	}
}

// GoexitTestSuite's goroutine fails fatally; the wait on it must end the
// method there.
type GoexitTestSuite struct{}

func (s *GoexitTestSuite) TestGoexit(t *gotest.T) {
	wait := gotest.Go(t, func() { gotest.Fail(t, "from the goroutine") })
	wait()
	fmt.Println("MARK:after wait")
}

// FuzzBrokenBeforeTestSuite's BeforeEach fails; AfterEach must still run for
// every fuzz execution.
type FuzzBrokenBeforeTestSuite struct{}

func (s *FuzzBrokenBeforeTestSuite) BeforeEach(t *gotest.T) { gotest.Fail(t, "before failed") }
func (s *FuzzBrokenBeforeTestSuite) AfterEach(t *gotest.T)  { fmt.Println("MARK:fuzz after broken") }
func (s *FuzzBrokenBeforeTestSuite) TestOne(t *gotest.T)    {}
func (s *FuzzBrokenBeforeTestSuite) FuzzOne(f *gotest.F) {
	f.Add("seed")
	f.Fuzz(func(t *gotest.T, in string) {})
}

// FuzzPanicBeforeTestSuite's BeforeEach panics; AfterEach must still run.
type FuzzPanicBeforeTestSuite struct{}

func (s *FuzzPanicBeforeTestSuite) BeforeEach(t *gotest.T) { panic("before panicked") }
func (s *FuzzPanicBeforeTestSuite) AfterEach(t *gotest.T)  { fmt.Println("MARK:fuzz after panic") }
func (s *FuzzPanicBeforeTestSuite) TestOne(t *gotest.T)    {}
func (s *FuzzPanicBeforeTestSuite) FuzzOne(f *gotest.F) {
	f.Add("seed")
	f.Fuzz(func(t *gotest.T, in string) {})
}

// PanicAheadTestSuite's BeforeEach panics and its AfterEach fails fatally:
// the panic must still be reported.
type PanicAheadTestSuite struct{}

func (s *PanicAheadTestSuite) BeforeEach(t *gotest.T) { panic("PANIC-AHEAD-OF-AFTEREACH") }
func (s *PanicAheadTestSuite) AfterEach(t *gotest.T) {
	fmt.Println("MARK:after the panic")
	gotest.Fail(t, "AfterEach fails")
}
func (s *PanicAheadTestSuite) TestOne(t *gotest.T) {}
func (s *PanicAheadTestSuite) BenchmarkOne(b *gotest.B) {
	for b.Loop() {
	}
}
func (s *PanicAheadTestSuite) FuzzOne(f *gotest.F) {
	f.Add("seed")
	f.Fuzz(func(t *gotest.T, in string) {})
}
