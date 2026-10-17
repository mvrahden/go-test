package panicking_test

import "github.com/mvrahden/go-test/pkg/gotest"

// PanickingTestSuite has a method that panics. The panic ends the test
// binary, so the sibling declared after it gets no verdict; the run is red
// regardless (see the canary's TestAPanicFailsTheMethod).
type PanickingTestSuite struct{}

func (s *PanickingTestSuite) TestPanics(t *gotest.T)  { panic("canary panic") }
func (s *PanickingTestSuite) TestSibling(t *gotest.T) { gotest.True(t, true) }
