package failing_test

import "github.com/mvrahden/go-test/pkg/gotest"

// FailingTestSuite fails on purpose: the package every failure-shaped
// output check runs.
type FailingTestSuite struct{}

func (s *FailingTestSuite) TestFails(t *gotest.T) { gotest.Fail(t, "deliberate") }
