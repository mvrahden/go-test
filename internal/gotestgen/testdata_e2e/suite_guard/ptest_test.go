package guardpkg

import (
	"os"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type GuardedTestSuite struct{}

func (s *GuardedTestSuite) SuiteGuard() string {
	if os.Getenv("ENABLE_GUARDED_SUITE") == "" {
		return "ENABLE_GUARDED_SUITE not set"
	}
	return ""
}

func (s *GuardedTestSuite) BeforeAll(t *gotest.T) {}

func (s *GuardedTestSuite) TestHello(t *gotest.T) { HelloWorld() }
func (s *GuardedTestSuite) TestWorld(t *gotest.T) { HelloWorld() }
