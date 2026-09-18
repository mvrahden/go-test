package asynchronous_test

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// AsyncTestSuite declares two async methods: one calls done() from another
// goroutine and passes, one never calls it and must fail at the deadline.
type AsyncTestSuite struct{}

func (s *AsyncTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Timeout = 300 * time.Millisecond
	return cfg
}

func (s *AsyncTestSuite) TestCallsDoneAsync(t *gotest.T, done func()) {
	go func() {
		time.Sleep(10 * time.Millisecond)
		done()
	}()
}

func (s *AsyncTestSuite) TestNeverDoneAsync(t *gotest.T, done func()) {}
