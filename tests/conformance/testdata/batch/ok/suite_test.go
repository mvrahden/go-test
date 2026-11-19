package ok_test

import "github.com/mvrahden/go-test/pkg/gotest"

// OkTestSuite builds and passes beside packages that fail vet.
type OkTestSuite struct{}

func (s *OkTestSuite) TestPasses(t *gotest.T) { gotest.True(t, true) }
