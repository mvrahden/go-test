package testpkg

import (
	"fmt"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// SetupOverrunTestSuite blows its SetupTimeout in BeforeAll and AfterAll while
// ignoring the context, so neither phase can be stopped, only judged.
type SetupOverrunTestSuite struct{}

func (s *SetupOverrunTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{SetupTimeout: 200 * time.Millisecond, Timeout: time.Minute}
}

func (s *SetupOverrunTestSuite) BeforeAll(t *gotest.T) {
	time.Sleep(1500 * time.Millisecond)
	fmt.Println("MARK:beforeall returned")
}

func (s *SetupOverrunTestSuite) AfterAll(t *gotest.T) {
	time.Sleep(500 * time.Millisecond)
	fmt.Println("MARK:afterall ran")
}

func (s *SetupOverrunTestSuite) TestOne(t *gotest.T) { fmt.Println("MARK:test ran") }
