// Package fzskiphook has a fuzz-only suite whose BeforeEach skips every
// execution; each execution must still see AfterEach.
package fzskiphook

import (
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// mark appends event to the markers file of the run's canary directory.
func mark(event string) {
	f, err := os.OpenFile(filepath.Join(os.Getenv("GOTEST_CANARY_DIR"), "markers"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(event + "\n")
}

// SkipBeforeTestSuite's BeforeEach skips every execution.
type SkipBeforeTestSuite struct{}

func (s *SkipBeforeTestSuite) BeforeEach(t *gotest.T) { t.Skipf("before skipped") }
func (s *SkipBeforeTestSuite) AfterEach(t *gotest.T)  { mark("after skip") }
func (s *SkipBeforeTestSuite) FuzzOne(f *gotest.F) {
	f.Add("seed")
	f.Fuzz(func(t *gotest.T, in string) { mark("body ran") })
}
