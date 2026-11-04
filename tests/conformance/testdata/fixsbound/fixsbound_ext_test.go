package fixsbound_test

import (
	"io"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/conformance/testdata/fixsbound"
)

// ExternalTestSuite binds the same package fixture from the external test
// package.
type ExternalTestSuite struct {
	Infra *fixsbound.InfraFixture
}

func (s *ExternalTestSuite) TestSeesTheSharedFixture(t *gotest.T) {
	base := s.Infra.Derived.Base
	gotest.Equal(t, "derived from "+base.Path, s.Infra.Derived.From)
	gotest.NotZero(t, base.Handle)
	b, err := io.ReadAll(base.Handle)
	gotest.NoError(t, err)
	gotest.Equal(t, "base-data", string(b))
}
