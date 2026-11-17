package aftereachdefault

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// SlowTestSuite declares no Timeout; its method sleeps twice the default
// deadline at a GOTEST_TIMEOUT_SCALE of 0.01 (300ms).
type SlowTestSuite struct{ Release *ReleaseFixture }

func (s *SlowTestSuite) TestSlow(t *gotest.T) { time.Sleep(600 * time.Millisecond) }
