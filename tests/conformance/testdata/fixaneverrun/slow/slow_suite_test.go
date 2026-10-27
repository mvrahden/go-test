package slow_test

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/conformance/testdata/fixaneverrun/fixtures"
)

// SlowTestSuite reads no fixture and runs past the others.
type SlowTestSuite struct{}

func (s *SlowTestSuite) TestTakesItsTime(t *gotest.T) {
	time.Sleep(4 * time.Second)
	fixtures.Mark("slow ends")
}
