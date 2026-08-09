package testpkg

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

var successAfterAllCalls atomic.Int32

// SuccessTestSuite exercises the happy path of a parallel suite: every test
// method must complete before AfterAll runs, and AfterAll must run exactly once
// with a live context.
type SuccessTestSuite struct{}

func (s *SuccessTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *SuccessTestSuite) BeforeAll(t *gotest.T) {
	fmt.Println("MARK:beforeall")
}

func (s *SuccessTestSuite) AfterAll(t *gotest.T) {
	fmt.Println("MARK:afterall calls=", successAfterAllCalls.Add(1), "ctxErr=", t.Context().Err())
}

func (s *SuccessTestSuite) TestAlpha(t *gotest.T) {
	time.Sleep(300 * time.Millisecond)
	fmt.Println("MARK:done alpha")
}

func (s *SuccessTestSuite) TestBeta(t *gotest.T) {
	time.Sleep(150 * time.Millisecond)
	fmt.Println("MARK:done beta")
}

func (s *SuccessTestSuite) TestGamma(t *gotest.T) {
	t.When("a nested behavior runs", func(w *gotest.T) {
		w.It("completes", func(it *gotest.T) {
			gotest.Equal(it, 2, 1+1)
		})
	})
	fmt.Println("MARK:done gamma")
}
