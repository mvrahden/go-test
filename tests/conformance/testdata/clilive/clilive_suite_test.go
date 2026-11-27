// Package clilive has two suites that each pass a first method and then hold
// in a second until the run under test releases them, so a test can read the
// stream while both suites still run.
package clilive

import (
	"os"
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type FirstTestSuite struct{}

func (s *FirstTestSuite) TestAPasses(t *gotest.T) { gotest.Equal(t, 2, 1+1) }
func (s *FirstTestSuite) TestBHolds(t *gotest.T)  { hold(t, "first") }

type SecondTestSuite struct{}

func (s *SecondTestSuite) TestAPasses(t *gotest.T) { gotest.Equal(t, 2, 1+1) }
func (s *SecondTestSuite) TestBHolds(t *gotest.T)  { hold(t, "second") }

// hold returns once the file name exists in LIVE_RELEASE_DIR, and at once
// without the variable: a run that only builds. It fails when its context
// ends first, unless LIVE_IGNORE_CONTEXT=1 has it hang until it is killed.
func hold(t *gotest.T, name string) {
	dir := os.Getenv("LIVE_RELEASE_DIR")
	if dir == "" {
		t.AssertsBy("a build-only run holds nothing")
		return
	}
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	ended := t.Context().Done()
	for {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			t.AssertsBy("released by the run under test")
			return
		}
		select {
		case <-ended:
			if os.Getenv("LIVE_IGNORE_CONTEXT") != "1" {
				gotest.Fail(t, "never released")
				return
			}
			ended = nil
		case <-tick.C:
		}
	}
}
