package stopatopen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// FirstTestSuite and SecondTestSuite each bind the fixture; neither runs
// when the run was stopped before the first opened.
type FirstTestSuite struct {
	Open *OpenFixture
}

func (s *FirstTestSuite) BeforeAll(t *gotest.T) { mark("first.BeforeAll") }
func (s *FirstTestSuite) AfterAll(t *gotest.T)  {}
func (s *FirstTestSuite) TestRuns(t *gotest.T)  { mark("first ran") }

type SecondTestSuite struct {
	Open *OpenFixture
}

func (s *SecondTestSuite) BeforeAll(t *gotest.T) { mark("second.BeforeAll") }
func (s *SecondTestSuite) AfterAll(t *gotest.T)  {}
func (s *SecondTestSuite) TestRuns(t *gotest.T)  { mark("second ran") }

// TestMain marks that it runs, then waits up to a minute for the go marker
// before it runs the tests.
func TestMain(m *testing.M) {
	mark("main")
	markers := filepath.Join(os.Getenv("GOTEST_CANARY_DIR"), "markers")
	for deadline := time.Now().Add(time.Minute); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if b, _ := os.ReadFile(markers); strings.Contains(string(b), "\ngo\n") {
			break
		}
	}
	os.Exit(m.Run())
}
