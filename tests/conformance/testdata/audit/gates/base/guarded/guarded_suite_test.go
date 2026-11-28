package guarded

import (
	"os"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type GuardedTestSuite struct{}

func (s *GuardedTestSuite) SuiteGuard() string {
	if os.Getenv("GATESMOD_DB") == "" {
		return "GATESMOD_DB is not set"
	}
	return ""
}

func (s *GuardedTestSuite) TestTwice(t *gotest.T) {
	gotest.Equal(t, 4, Twice(2))
}
