package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type myCtx struct{ val string }

type MyTestSuite struct{}

func (s *MyTestSuite) BeforeEach(t *gotest.T) *myCtx { return &myCtx{} }
func (s *MyTestSuite) AfterEach(t *gotest.T, ctx *myCtx) {}
func (s *MyTestSuite) TestOne(t *gotest.T, ctx *myCtx) {}
