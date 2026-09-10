package panicking_test

import "github.com/mvrahden/go-test/pkg/gotest"

// PanickingTestSuite has a method that panics and a sibling that must still
// run and pass: the panic is contained to its method.
type PanickingTestSuite struct{}

func (s *PanickingTestSuite) TestPanics(t *gotest.T)  { panic("canary panic") }
func (s *PanickingTestSuite) TestSibling(t *gotest.T) { gotest.True(t, true) }
