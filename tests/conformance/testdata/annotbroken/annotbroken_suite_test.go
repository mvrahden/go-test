package annotbroken

import "github.com/mvrahden/go-test/pkg/gotest"

type BrokenTestSuite struct{}

func (s *BrokenTestSuite) TestNothing(t *gotest.T) { _ = undefinedName }
