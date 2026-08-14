package exclusive

import (
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/sharedfixture/fixtures"
)

// ExclusiveDeltaTestSuite runs in the serial tail and is the only suite that
// references DeltaSharedFixture. The runner therefore defers Delta past the
// parallel bulk and starts it at the bulk→tail barrier — this suite passing
// in a whole-repo run is the end-to-end proof of window scheduling.
type ExclusiveDeltaTestSuite struct {
	Delta *fixtures.DeltaSharedFixture
}

func (s *ExclusiveDeltaTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Exclusive: true}
}

func (s *ExclusiveDeltaTestSuite) TestDeltaAvailable(t *gotest.T) {
	gotest.Equal(t, "delta-shared", s.Delta.Stamp)
}
