package uses

import "github.com/mvrahden/go-test/pkg/gotest"

type UsesTestSuite struct{ name string }

func (s *UsesTestSuite) TestName(t *gotest.T) { gotest.Empty(t, s.name) }
