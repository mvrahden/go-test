package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type MyTestSuite struct{}

func (s MyTestSuite) TestOne(t *gotest.T) {}
