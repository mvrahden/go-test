package aftereach_test

import (
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// AfterEachTestSuite proves teardown runs when the test failed: AfterEach
// writes its marker after a method that fails.
type AfterEachTestSuite struct{}

func (s *AfterEachTestSuite) AfterEach(t *gotest.T) {
	_ = os.WriteFile(filepath.Join(os.Getenv("GOTEST_CANARY_DIR"), "aftereach-ran"), []byte("ran"), 0o600)
}

func (s *AfterEachTestSuite) TestFailsBeforeTeardown(t *gotest.T) { gotest.True(t, false) }
