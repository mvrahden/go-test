package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type myCtx struct{}

type MyTestSuite struct{}

func (s *MyTestSuite) BeforeEach(t *gotest.T) *myCtx { return &myCtx{} }
func (s *MyTestSuite) AfterEach(t *gotest.T) {}
func (s *MyTestSuite) TestOne(t *gotest.T, ctx *myCtx) {}
