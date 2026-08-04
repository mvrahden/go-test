package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type myTestSuite struct{}

func (s *myTestSuite) TestOne(t *gotest.T) {}
