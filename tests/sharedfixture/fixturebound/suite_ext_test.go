package fixturebound_test

import (
	"io"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/sharedfixture/fixturebound"
)

// ServiceExtTestSuite binds the same fixture from the external test package,
// so a run over this package proves fixture bindings reach both overlays.
type ServiceExtTestSuite struct {
	Infra *fixturebound.InfraFixture
}

func (s *ServiceExtTestSuite) TestAlphaViaFixtureFromExt(t *gotest.T) {
	gotest.NotEmpty(t, s.Infra.Alpha.Handle)
	_, err := s.Infra.Alpha.Handle.Seek(0, 0)
	gotest.NoError(t, err)
	b, err := io.ReadAll(s.Infra.Alpha.Handle)
	gotest.NoError(t, err)
	gotest.Equal(t, "alpha-data", string(b))
}
