package parallelsetenvok

import (
	"os"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// SetupEnvTestSuite is parallel and sets the environment in BeforeAll alone.
type SetupEnvTestSuite struct{}

func (s *SetupEnvTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *SetupEnvTestSuite) BeforeAll(t *gotest.T) { t.Setenv("PARALLELSETENVOK", "1") }

func (s *SetupEnvTestSuite) AfterAll(t *gotest.T) {}

func (s *SetupEnvTestSuite) TestReadsA(t *gotest.T) {
	gotest.Equal(t, "1", os.Getenv("PARALLELSETENVOK"))
}

func (s *SetupEnvTestSuite) TestReadsB(t *gotest.T) {
	gotest.Equal(t, "1", os.Getenv("PARALLELSETENVOK"))
}
