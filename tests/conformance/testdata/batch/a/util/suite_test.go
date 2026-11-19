package util_test

import "github.com/mvrahden/go-test/pkg/gotest"

// UtilATestSuite shares its package's last path element with another.
type UtilATestSuite struct{}

func (s *UtilATestSuite) TestPasses(t *gotest.T) { gotest.True(t, true) }
