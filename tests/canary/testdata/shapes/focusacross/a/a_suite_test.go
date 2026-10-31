package a

import (
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// F_OnlyTestSuite is the run's only focused suite.
type F_OnlyTestSuite struct{}

func (s *F_OnlyTestSuite) TestRuns(t *gotest.T) {
	w, err := os.OpenFile(filepath.Join(os.Getenv("GOTEST_CANARY_DIR"), "markers"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	gotest.NoError(t, err)
	_, err = w.WriteString("focused ran\n")
	gotest.NoError(t, err)
	gotest.NoError(t, w.Close())
}
