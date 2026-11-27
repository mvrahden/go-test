// Package snapfailed matches snapshots in tests that have already failed:
// Each rows and methods that fail before their MatchSnapshot, beside a row
// that passes.
package snapfailed

import (
	"os"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type SnapFailedTestSuite struct{}

type row struct {
	Desc string
	ok   bool
}

func (s *SnapFailedTestSuite) TestRows(t *gotest.T) {
	for it, r := range gotest.Each(t, []row{{"passing", true}, {"kept", false}, {"fresh", false}}) {
		gotest.True(it, r.ok, "the %s row fails", r.Desc)
		gotest.MatchSnapshot(it, os.Getenv("SNAP_VALUE"))
	}
}

func (s *SnapFailedTestSuite) TestAMethod(t *gotest.T) {
	t.Errorf("the method fails and goes on")
	gotest.MatchSnapshot(t, os.Getenv("SNAP_VALUE"))
}

func (s *SnapFailedTestSuite) TestAFreshMethod(t *gotest.T) {
	t.Errorf("the method fails and goes on")
	gotest.MatchSnapshot(t, os.Getenv("SNAP_VALUE"))
}
