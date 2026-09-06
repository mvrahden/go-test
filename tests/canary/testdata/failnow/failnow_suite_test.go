package failnow_test

import (
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// FailNowTestSuite proves an assertion halts the method: the marker written
// after the failing assertion must never exist.
type FailNowTestSuite struct{}

func (s *FailNowTestSuite) TestHaltsAtTheFailure(t *gotest.T) {
	gotest.Equal(t, 1, 2)
	_ = os.WriteFile(filepath.Join(os.Getenv("GOTEST_CANARY_DIR"), "after-failnow"), []byte("reached"), 0o600)
}
