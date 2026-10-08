package quick

import (
	"os"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// QuickTestSuite returns at once, so its hold reaches the teardown.
type QuickTestSuite struct {
	DB *DBFixture
}

func (s *QuickTestSuite) TestQuick(t *gotest.T) {}

// EnvTestSuite reports what a test sees of the runner's stop file.
type EnvTestSuite struct{}

func (s *EnvTestSuite) TestEnv(t *gotest.T) {
	if on("STOPPING_ENV") {
		logLine("env=" + os.Getenv("GOTEST_STOP_FILE"))
	}
}
