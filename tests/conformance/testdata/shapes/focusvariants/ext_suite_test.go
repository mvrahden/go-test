package focusvariants_test

import "github.com/mvrahden/go-test/pkg/gotest"

// ExtTestSuite is unfocused, in the external test package.
type ExtTestSuite struct{}

func (s *ExtTestSuite) TestRuns(t *gotest.T) {}
