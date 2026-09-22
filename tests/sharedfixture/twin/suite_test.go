package twin

import (
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/sharedfixture/fixtures"
)

// AlphaTestSuite shares its name with standalone's AlphaTestSuite and needs
// a different shared fixture: the two must never read each other's state.
type AlphaTestSuite struct {
	Beta *fixtures.BetaSharedFixture
}

func (s *AlphaTestSuite) TestBetaOnly(t *gotest.T) {
	gotest.Equal(t, "beta-shared", s.Beta.Label)
	gotest.Equal(t, 42, s.Beta.Count)
}
