package groupfixtures

import (
	"os"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type ADefaultTestSuite struct{}

func (s *ADefaultTestSuite) TestRuns(t *gotest.T) {
	mark("default begins")
	time.Sleep(300 * time.Millisecond)
	mark("default ends")
}

// E2EStackTestSuite is the only reader of the stack.
type E2EStackTestSuite struct {
	Stack *StackSharedFixture
}

func (s *E2EStackTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{ExclusiveGroup: "e2e"}
}

func (s *E2EStackTestSuite) TestReadsStack(t *gotest.T) {
	gotest.Equal(t, "up", s.Stack.State)
	if os.Getenv("GROUPFIXTURES_HOLD") != "" {
		mark("e2e stack holds")
		select {
		case <-t.Context().Done():
		case <-time.After(20 * time.Second):
		}
	}
	mark("e2e stack ran")
}

// E2ESlowTestSuite outlasts the stack's reader, or holds until stopped.
type E2ESlowTestSuite struct{}

func (s *E2ESlowTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{ExclusiveGroup: "e2e"}
}

func (s *E2ESlowTestSuite) TestHolds(t *gotest.T) {
	mark("e2e slow begins")
	select {
	case <-t.Context().Done():
	case <-time.After(2 * time.Second):
	}
	mark("e2e slow ends")
}

type LoadTestSuite struct {
	Load *LoadSharedFixture
}

func (s *LoadTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{ExclusiveGroup: "load"}
}

func (s *LoadTestSuite) TestReadsLoad(t *gotest.T) {
	gotest.Equal(t, "up", s.Load.State)
	mark("load ran")
}
