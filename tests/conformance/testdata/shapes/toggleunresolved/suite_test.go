package toggleunresolved

import "github.com/mvrahden/go-test/pkg/gotest"

type UnresolvedTestSuite struct{}

func (s *UnresolvedTestSuite) TestOne(t *gotest.T) {}
