package clirerun_test

import "github.com/mvrahden/go-test/pkg/gotest"

// RerunTestSuite has one method that fails and one that passes: --failed
// reruns the first alone.
type RerunTestSuite struct{}

func (s *RerunTestSuite) TestFails(t *gotest.T) { gotest.Fail(t, "deliberate") }

func (s *RerunTestSuite) TestPasses(t *gotest.T) { gotest.True(t, true) }
