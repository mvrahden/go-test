package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

// A focused suite skips every other suite, but a helper is no suite to skip.

type HelperTestSuite struct{}

type OtherTestSuite struct{}

func (s *OtherTestSuite) TestOne(t *gotest.T) {}

type F_FocusedTestSuite struct{}

func (s *F_FocusedTestSuite) TestOne(t *gotest.T) {}
