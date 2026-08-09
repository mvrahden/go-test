package testpkg

import (
	"fmt"
	"strconv"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// MustErrorTestSuite reproduces the originally reported failure: gotest.Must
// raises a raw panic on a non-nil error from inside a nested behavior of a
// parallel test method.
type MustErrorTestSuite struct{}

func (s *MustErrorTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *MustErrorTestSuite) AfterAll(t *gotest.T) {
	fmt.Println("MARK:afterall")
}

func (s *MustErrorTestSuite) TestMustOnError(t *gotest.T) {
	t.When("a nested behavior unwraps a failing call", func(w *gotest.T) {
		w.It("panics through gotest.Must", func(it *gotest.T) {
			n := gotest.Must(strconv.Atoi("not-a-number"))
			fmt.Println("MARK:unreachable", n)
		})
	})
}

func (s *MustErrorTestSuite) TestSlowSibling(t *gotest.T) {
	time.Sleep(2 * time.Second)
	fmt.Println("MARK:done sibling")
}
