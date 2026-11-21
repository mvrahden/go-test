package procbarrier_test

import (
	"os"
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// Three suites, three processes: each marks its arrival and waits for the
// other two, so they pass only when all three run at once.

// meet marks name's arrival in PROCBARRIER_DIR and waits for three.
func meet(t *gotest.T, name string) {
	dir := os.Getenv("PROCBARRIER_DIR")
	gotest.NoError(t, os.WriteFile(filepath.Join(dir, name), nil, 0o600))
	gotest.Eventually(t, 3*time.Second, 20*time.Millisecond, func(poll *gotest.R) {
		entries, err := os.ReadDir(dir)
		gotest.NoError(poll, err)
		gotest.Len(poll, entries, 3)
	})
}

type MeetATestSuite struct{}

func (s *MeetATestSuite) TestMeets(t *gotest.T) { meet(t, "A") }

type MeetBTestSuite struct{}

func (s *MeetBTestSuite) TestMeets(t *gotest.T) { meet(t, "B") }

type MeetCTestSuite struct{}

func (s *MeetCTestSuite) TestMeets(t *gotest.T) { meet(t, "C") }
