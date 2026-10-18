package setupwaits

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// DerivedTestSuite starts its goroutine on a T derived by NewTWithDeadline;
// only AfterAll stops it.
type DerivedTestSuite struct{ stop chan struct{} }

func (s *DerivedTestSuite) BeforeAll(t *gotest.T) {
	s.stop = make(chan struct{})
	dt := gotest.NewTWithDeadline(t, time.Minute)
	gotest.Go(dt, func() { <-s.stop })
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
