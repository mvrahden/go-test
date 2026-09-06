package lifecycle_test

import (
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// LifecycleTestSuite records the order of BeforeAll, the methods and AfterAll
// into a file the canary reads back.
type LifecycleTestSuite struct{}

func record(event string) {
	path := filepath.Join(os.Getenv("GOTEST_CANARY_DIR"), "lifecycle")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(event + "\n")
}

func (s *LifecycleTestSuite) BeforeAll(t *gotest.T) { record("beforeAll") }
func (s *LifecycleTestSuite) AfterAll(t *gotest.T)  { record("afterAll") }
func (s *LifecycleTestSuite) TestFirst(t *gotest.T) { record("TestFirst") }
func (s *LifecycleTestSuite) TestSecond(t *gotest.T) {
	record("TestSecond")
}
