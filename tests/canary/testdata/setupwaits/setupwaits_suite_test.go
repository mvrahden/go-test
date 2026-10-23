package setupwaits

import (
	"testing"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// DerivedTestSuite starts its goroutines on Ts derived from BeforeAll's: by
// NewTWithDeadline, When, It and Each. Only AfterAll stops them.
type DerivedTestSuite struct{ stop chan struct{} }

func (s *DerivedTestSuite) BeforeAll(t *gotest.T) {
	s.stop = make(chan struct{})
	gotest.Go(gotest.NewTWithDeadline(t, time.Minute), func() { <-s.stop })
	t.When("serving", func(w *gotest.T) {
		gotest.Go(w, func() { <-s.stop })
		w.It("in an It", func(it *gotest.T) { gotest.Go(it, func() { <-s.stop }) })
		for row := range gotest.Each(w, []string{"row"}) {
			gotest.Go(row, func() { <-s.stop })
		}
	})
}

func (s *DerivedTestSuite) AfterAll(t *gotest.T) { close(s.stop) }

func (s *DerivedTestSuite) TestServed(t *gotest.T) {}

// FailLastTestSuite's BeforeAll starts a slow goroutine, then one that
// fails at once.
type FailLastTestSuite struct {
	Fixture *FailFixture
}

func (s *FailLastTestSuite) BeforeAll(t *gotest.T) {
	gotest.Go(t, func() {
		time.Sleep(time.Second)
		mark("fail: first goroutine ended")
	})
	gotest.Go(t, func() { gotest.Fail(t, "the last goroutine fails") })
}

func (s *FailLastTestSuite) AfterAll(t *gotest.T) {}

func (s *FailLastTestSuite) TestOne(t *gotest.T) {}

// PanicLastTestSuite's BeforeAll starts a slow goroutine, then one that
// panics at once.
type PanicLastTestSuite struct {
	Fixture *PanicFixture
}

func (s *PanicLastTestSuite) BeforeAll(t *gotest.T) {
	gotest.Go(t, func() {
		time.Sleep(time.Second)
		mark("panic: first goroutine ended")
	})
	gotest.Go(t, func() { panic("the last goroutine panics") })
}

func (s *PanicLastTestSuite) AfterAll(t *gotest.T) {}

func (s *PanicLastTestSuite) TestOne(t *gotest.T) {}

// StdTestSuite's BeforeAll takes a *testing.T and wraps it; only AfterAll
// stops its goroutines.
type StdTestSuite struct{ stop chan struct{} }

func (s *StdTestSuite) BeforeAll(t *testing.T) {
	s.stop = make(chan struct{})
	gotest.Go(gotest.NewT(t), func() { <-s.stop })
	gotest.Go(gotest.NewTWithDeadline(t, time.Minute), func() { <-s.stop })
}

func (s *StdTestSuite) AfterAll(t *testing.T) { close(s.stop) }

func (s *StdTestSuite) TestServed(t *testing.T) {}

// LateTestSuite's goroutine starts another once AfterAll stopped it; that
// one panics.
type LateTestSuite struct{ stop chan struct{} }

func (s *LateTestSuite) BeforeAll(t *gotest.T) {
	s.stop = make(chan struct{})
	gotest.Go(t, func() {
		<-s.stop
		gotest.Go(t, func() {
			time.Sleep(100 * time.Millisecond)
			panic("LATE-GOROUTINE-PANIC")
		})
	})
}

func (s *LateTestSuite) AfterAll(t *gotest.T) { close(s.stop) }

func (s *LateTestSuite) TestServed(t *gotest.T) {}

// SlowTestSuite's goroutine outlives its SetupTimeout, which AfterAll does
// nothing about; it ends on its own long after.
type SlowTestSuite struct{}

func (s *SlowTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{SetupTimeout: 200 * time.Millisecond}
}

func (s *SlowTestSuite) BeforeAll(t *gotest.T) {
	gotest.Go(t, func() { time.Sleep(4 * time.Second) })
}

func (s *SlowTestSuite) AfterAll(t *gotest.T) {}

func (s *SlowTestSuite) TestServed(t *gotest.T) {}
