package fixsbound

import (
	"io"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// InternalTestSuite binds the package fixture from the internal test package.
type InternalTestSuite struct {
	Infra *InfraFixture
}

func (s *InternalTestSuite) TestSeesTheSharedFixture(t *gotest.T) {
	base := s.Infra.Derived.Base
	gotest.Equal(t, "derived from "+base.Path, s.Infra.Derived.From)
	gotest.NotZero(t, base.Handle)
	b, err := io.ReadAll(base.Handle)
	gotest.NoError(t, err)
	gotest.Equal(t, "base-data", string(b))
}
