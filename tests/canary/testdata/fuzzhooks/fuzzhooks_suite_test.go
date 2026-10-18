// Package fuzzhooks has fuzz-only suites whose BeforeEach fails or panics;
// every execution must still see AfterEach.
package fuzzhooks

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/mvrahden/go-test/pkg/gotest"
)

var markMu sync.Mutex

// mark appends event to the markers file of the run's canary directory.
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

// BrokenBeforeTestSuite's BeforeEach fails every execution.
type BrokenBeforeTestSuite struct{}

func (s *BrokenBeforeTestSuite) BeforeEach(t *gotest.T) { gotest.Fail(t, "before failed") }
func (s *BrokenBeforeTestSuite) AfterEach(t *gotest.T)  { mark("after broken") }
func (s *BrokenBeforeTestSuite) FuzzOne(f *gotest.F) {
	f.Add("seed")
	f.Fuzz(func(t *gotest.T, in string) {})
}

// PanicBeforeTestSuite's BeforeEach panics every execution.
type PanicBeforeTestSuite struct{}

func (s *PanicBeforeTestSuite) BeforeEach(t *gotest.T) { panic("before panicked") }
func (s *PanicBeforeTestSuite) AfterEach(t *gotest.T)  { mark("after panic") }
func (s *PanicBeforeTestSuite) FuzzOne(f *gotest.F) {
	f.Add("seed")
	f.Fuzz(func(t *gotest.T, in string) {})
}
