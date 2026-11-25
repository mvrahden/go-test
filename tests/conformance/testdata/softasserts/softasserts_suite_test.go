// Package softasserts fails soft assertions in a method, a helper, an It, a
// hook and a poll, and marks what runs after them.
package softasserts

import (
	"os"
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// mark appends one line to the markers file the run under test reads back.
func mark(event string) {
	f, err := os.OpenFile(filepath.Join(os.Getenv("GOTEST_CANARY_DIR"), "markers"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(event + "\n")
}

type SoftTestSuite struct{}

func (s *SoftTestSuite) TestMethod(t *gotest.T) {
	gotest.Equal(t.Soft(), 1, 2)
	gotest.True(gotest.NewTWithDeadline(t.Soft(), time.Minute), false)
	mark("after the soft failures")
}

func (s *SoftTestSuite) TestHelper(t *gotest.T) {
	expectAnswer(t.Soft(), 41)
	gotest.Equal(t.Soft(), "went on", "after the helper")
}

func expectAnswer(t *gotest.T, got int) {
	gotest.Equal(t, 42, got)
}

func (s *SoftTestSuite) TestIt(t *gotest.T) {
	t.Soft().It("stops", func(it *gotest.T) {
		gotest.True(it, false)
		mark("after the failure in the It")
	})
	mark("after the It")
}

func (s *SoftTestSuite) TestEventually(t *gotest.T) {
	gotest.Eventually(t.Soft(), 50*time.Millisecond, 10*time.Millisecond, func(poll *gotest.R) {
		gotest.True(poll, false)
	})
	mark("after Eventually")
}

type SoftHookTestSuite struct{}

func (s *SoftHookTestSuite) BeforeEach(t *gotest.T) {
	gotest.True(t.Soft(), false, "BeforeEach fails")
	mark("BeforeEach went on")
}

func (s *SoftHookTestSuite) TestRuns(t *gotest.T) { mark("the method ran") }
