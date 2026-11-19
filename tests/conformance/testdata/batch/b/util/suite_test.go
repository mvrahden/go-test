package util_test

import "github.com/mvrahden/go-test/pkg/gotest"

// UtilBTestSuite shares its package's last path element with another.
type UtilBTestSuite struct{}

func (s *UtilBTestSuite) TestPasses(t *gotest.T) { gotest.True(t, true) }
