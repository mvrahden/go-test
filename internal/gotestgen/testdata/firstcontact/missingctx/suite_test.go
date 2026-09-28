package missingctx

import "github.com/mvrahden/go-test/pkg/gotest"

type caseCtx struct{}

type CtxTestSuite struct{}

func (s *CtxTestSuite) BeforeEach(t *gotest.T) *caseCtx { return &caseCtx{} }

func (s *CtxTestSuite) TestWithContext(t *gotest.T, ctx *caseCtx) {}

func (s *CtxTestSuite) TestWithoutContext(t *gotest.T) {}
