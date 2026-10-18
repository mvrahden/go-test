package markers_test

import (
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// MarkerTestSuite stamps its lifecycle into the markers file and has one
// passing and one failing method, so a run of it exercises every field the
// oracles report: exit code, per-test verdicts and marker order.
type MarkerTestSuite struct{}

func mark(event string) {
	path := filepath.Join(os.Getenv("GOTEST_CANARY_DIR"), "markers")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(event + "\n")
}

func (s *MarkerTestSuite) BeforeAll(t *gotest.T) { mark("beforeAll") }
func (s *MarkerTestSuite) AfterAll(t *gotest.T)  { mark("afterAll") }
func (s *MarkerTestSuite) TestOne(t *gotest.T)   { mark("TestOne") }
func (s *MarkerTestSuite) TestTwo(t *gotest.T)   { gotest.Fail(t, "deliberate") }
