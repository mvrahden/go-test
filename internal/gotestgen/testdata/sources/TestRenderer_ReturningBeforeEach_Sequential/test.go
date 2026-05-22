package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type myCtx struct{ val string }

type OrderTestSuite struct{}

func (s *OrderTestSuite) BeforeEach(t *gotest.T) *myCtx { return &myCtx{} }
func (s *OrderTestSuite) AfterEach(t *gotest.T, ctx *myCtx) {}
func (s *OrderTestSuite) TestOne(t *gotest.T, ctx *myCtx) {}
func (s *OrderTestSuite) TestTwo(t *gotest.T, _ *myCtx)   {}
