package liveholds_test

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

// hold returns once the file name exists in LIVE_RELEASE_DIR, at once
// without the variable, and fails when its context ends first.
func hold(t *gotest.T, name string) {
	dir := os.Getenv("LIVE_RELEASE_DIR")
	if dir == "" {
		t.AssertsBy("nothing to hold for")
		return
	}
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			t.AssertsBy("released by the test")
			return
		}
		select {
		case <-t.Context().Done():
			gotest.Fail(t, "never released")
			return
		case <-tick.C:
		}
	}
}
