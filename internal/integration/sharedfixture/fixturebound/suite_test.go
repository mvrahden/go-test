package fixturebound

import (
	"io"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type ServiceTestSuite struct {
	Infra *InfraFixture
}

func (s *ServiceTestSuite) TestAlphaViaFixture(t *gotest.T) {
	gotest.NotEqual(t, "", s.Infra.Alpha.DataPath)
	gotest.NotEmpty(t, s.Infra.Alpha.Handle)
	_, err := s.Infra.Alpha.Handle.Seek(0, 0)
	gotest.NoError(t, err)
	b, err := io.ReadAll(s.Infra.Alpha.Handle)
	gotest.NoError(t, err)
	gotest.Equal(t, "alpha-data", string(b))
}
