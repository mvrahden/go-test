package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type myCtx struct{}

type BadTestSuite struct{}

func (s *BadTestSuite) AfterEach(t *gotest.T, ctx *myCtx, extra int) {}
func (s *BadTestSuite) TestOne(t *gotest.T) {}
