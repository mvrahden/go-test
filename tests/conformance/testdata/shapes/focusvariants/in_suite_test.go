package focusvariants

import "github.com/mvrahden/go-test/pkg/gotest"

// F_InTestSuite is focused, in the internal test package.
type F_InTestSuite struct{}

func (s *F_InTestSuite) TestRuns(t *gotest.T) {}
