// Package lcbreader has a method that outlives the reader of its stdout,
// for a run outside the gotest runner.
package lcbreader

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

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

// ReaderTestSuite's method waits two seconds, past several checks of its
// stream, unless its context ends first.
type ReaderTestSuite struct{}

func (s *ReaderTestSuite) BeforeAll(t *gotest.T) {}
func (s *ReaderTestSuite) AfterAll(t *gotest.T)  { mark("suite.AfterAll") }

func (s *ReaderTestSuite) TestOutlivesTheReader(t *gotest.T) {
	mark("running")
	select {
	case <-t.Context().Done():
		mark("context ended")
	case <-time.After(2 * time.Second):
		mark("not stopped")
	}
}
