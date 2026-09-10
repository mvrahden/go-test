package broken_test

import "github.com/mvrahden/go-test/pkg/gotest"

// BrokenTestSuite does not compile: the canary expects exit 2 and no verdict.
type BrokenTestSuite struct{}

func (s *BrokenTestSuite) TestNeverBuilt(t *gotest.T) { gotest.Equal(t, 1, undefinedIdentifier) }
