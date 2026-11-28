package nomrun_test

import (
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// TestMain returns without calling m.Run: the binary exits 0 having run
// no test, which only the census tells from a pass.
func TestMain(m *testing.M) {}

// NoMRunTestSuite never runs: its binary's TestMain skips m.Run.
type NoMRunTestSuite struct{}

func (s *NoMRunTestSuite) TestNeverRuns(t *gotest.T) { gotest.True(t, true) }
