package testpkg

import (
	"fmt"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// PanicTestSuite panics directly in the body of a parallel test method while a
// sibling method is still in flight.
type PanicTestSuite struct{}

func (s *PanicTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *PanicTestSuite) AfterAll(t *gotest.T) {
	fmt.Println("MARK:afterall")
}

func (s *PanicTestSuite) TestPanics(t *gotest.T) {
	panic("boom from a parallel test method")
}

func (s *PanicTestSuite) TestSlowSibling(t *gotest.T) {
	time.Sleep(2 * time.Second)
	fmt.Println("MARK:done sibling")
}
