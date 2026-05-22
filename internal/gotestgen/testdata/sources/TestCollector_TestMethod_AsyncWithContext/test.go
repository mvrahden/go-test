package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type myCtx struct{}

type MyTestSuite struct{}

func (s *MyTestSuite) BeforeEach(t *gotest.T) *myCtx { return &myCtx{} }
func (s *MyTestSuite) TestOneAsync(t *gotest.T, ctx *myCtx, done func()) {}
