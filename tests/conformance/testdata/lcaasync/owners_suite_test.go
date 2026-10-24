package lcaasync

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// OwnersTestSuite: in every method a helper goroutine fails at once while
// an owner of done the body did not start works on; AfterEach marks when
// the wait ended.
type OwnersTestSuite struct{ pool chan func() }

func (s *OwnersTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Timeout = 5 * time.Second
	return cfg
}

// BeforeAll starts a pool that serves every method, each job on a goroutine
// of the pool's.
func (s *OwnersTestSuite) BeforeAll(t *gotest.T) {
	s.pool = make(chan func())
	go func() {
		for job := range s.pool {
			go job()
		}
	}()
}

func (s *OwnersTestSuite) AfterAll(t *gotest.T) { close(s.pool) }

func (s *OwnersTestSuite) AfterEach(t *gotest.T) { mark("AfterEach " + t.T().Name()) }

// TestPoolOwnerFailsAsync's owner is a job of the pool's that fails late.
func (s *OwnersTestSuite) TestPoolOwnerFailsAsync(t *gotest.T, done func()) {
	helperFails(t)
	s.pool <- func() {
		time.Sleep(700 * time.Millisecond)
		mark("pool owner failed")
		gotest.Fail(t, "the pool owner fails late")
		done()
	}
}

// TestAfterFuncOwnerAsync's owner is a time.AfterFunc callback.
func (s *OwnersTestSuite) TestAfterFuncOwnerAsync(t *gotest.T, done func()) {
	helperFails(t)
	time.AfterFunc(700*time.Millisecond, func() {
		mark("AfterFunc owner called done")
		done()
	})
}

// BeforeEachGoroutineTestSuite's BeforeEach starts a goroutine that fails
// late; the owner of done fails at once.
type BeforeEachGoroutineTestSuite struct{}

func (s *BeforeEachGoroutineTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Timeout = 5 * time.Second
	return cfg
}

func (s *BeforeEachGoroutineTestSuite) BeforeEach(t *gotest.T) {
	go func() {
		time.Sleep(700 * time.Millisecond)
		mark("BeforeEach's goroutine failed")
		gotest.Fail(t, "BeforeEach's goroutine fails late")
	}()
}

func (s *BeforeEachGoroutineTestSuite) AfterEach(t *gotest.T) { mark("AfterEach " + t.T().Name()) }

func (s *BeforeEachGoroutineTestSuite) TestOwnerFailsAsync(t *gotest.T, done func()) {
	gotest.Go(t, func() { gotest.Fail(t, "the owner of done fails") })
}

// PanicTestSuite's owner of done panics on a goroutine gotest.Go started; a
// generous Timeout, so a wait that ends early ends for a reason of its own.
type PanicTestSuite struct{}

func (s *PanicTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Timeout: 20 * time.Second}
}

func (s *PanicTestSuite) AfterEach(t *gotest.T) { mark("AfterEach " + t.T().Name()) }

func (s *PanicTestSuite) TestGoPanicsAsync(t *gotest.T, done func()) {
	gotest.Go(t, func() { panic("the owner of done panics") })
}

// ParallelTestSuite: one method's done is called by a goroutine whose
// creators ended, long after its parallel sibling's owner failed.
type ParallelTestSuite struct{}

func (s *ParallelTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Timeout = 10 * time.Second
	cfg.Parallel = true
	return cfg
}

func (s *ParallelTestSuite) AfterEach(t *gotest.T) { mark("AfterEach " + t.T().Name()) }

func (s *ParallelTestSuite) TestOrphanCallsDoneAsync(t *gotest.T, done func()) {
	go func() {
		time.Sleep(100 * time.Millisecond)
		go func() {
			go func() {
				time.Sleep(2 * time.Second)
				mark("orphan called done")
				done()
			}()
		}()
	}()
}

func (s *ParallelTestSuite) TestOwnerFailsAsync(t *gotest.T, done func()) {
	go func() {
		time.Sleep(300 * time.Millisecond)
		gotest.Fail(t, "the owner of done fails")
	}()
}
