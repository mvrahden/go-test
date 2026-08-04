package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type myCtx struct{}

type MyTestSuite struct{}

func (s *MyTestSuite) TestOne(t *gotest.T, ctx *myCtx) {}
