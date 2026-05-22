package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type GotestTestSuite struct{}

func (s *GotestTestSuite) TestFoo(t *gotest.T) {}
