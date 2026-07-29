package testpkg

import (
	"fmt"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// AssertionFailureTestSuite fails a nested behavior through the assertion path
// (FailNow / runtime.Goexit) rather than a panic, and skips another method.
// Both must still let AfterAll run and the process exit promptly.
type AssertionFailureTestSuite struct{}

func (s *AssertionFailureTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *AssertionFailureTestSuite) AfterAll(t *gotest.T) {
	fmt.Println("MARK:afterall")
}

func (s *AssertionFailureTestSuite) TestFailsAnAssertion(t *gotest.T) {
	t.When("a nested behavior asserts", func(w *gotest.T) {
		w.It("fails", func(it *gotest.T) {
			gotest.Equal(it, 1, 2)
		})
	})
}

func (s *AssertionFailureTestSuite) TestSkips(t *gotest.T) {
	t.Skipf("nothing to do here")
}

func (s *AssertionFailureTestSuite) TestPasses(t *gotest.T) {
	fmt.Println("MARK:done passes")
}
