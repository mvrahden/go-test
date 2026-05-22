package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type myCtx struct{}
type otherCtx struct{}

type MyTestSuite struct{}

func (s *MyTestSuite) BeforeEach(t *gotest.T) *myCtx { return &myCtx{} }
func (s *MyTestSuite) TestOne(t *gotest.T, ctx *otherCtx) {}
