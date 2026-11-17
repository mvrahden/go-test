// Package stopended is a suite whose first method returns at once and whose
// second holds until its context ends; a run of it is stopped from outside.
package stopended

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// StopEndedTestSuite: one method done before the stop, one the stop ends.
type StopEndedTestSuite struct{}

func (s *StopEndedTestSuite) TestAReturnsFirst(t *gotest.T) { mark("returned") }

func (s *StopEndedTestSuite) TestHolds(t *gotest.T) {
	mark("running")
	<-t.Context().Done()
	mark("context ended")
}

var markMu sync.Mutex

// mark appends one line to the markers file the run under test reads back.
func mark(event string) {
	markMu.Lock()
	defer markMu.Unlock()
	f, err := os.OpenFile(filepath.Join(os.Getenv("GOTEST_CANARY_DIR"), "markers"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(event + "\n")
}
