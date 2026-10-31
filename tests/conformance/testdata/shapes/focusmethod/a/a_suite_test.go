package a

import "github.com/mvrahden/go-test/pkg/gotest"

// MethodTestSuite is unfocused and holds the run's only focused method.
type MethodTestSuite struct{}

func (s *MethodTestSuite) F_TestFocused(t *gotest.T) {}

func (s *MethodTestSuite) TestPlain(t *gotest.T) {}
