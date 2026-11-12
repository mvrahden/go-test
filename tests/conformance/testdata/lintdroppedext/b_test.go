package lintdroppedext_test

import (
	"github.com/mvrahden/go-test/pkg/gotest"
	p "github.com/mvrahden/go-test/tests/conformance/testdata/lintdroppedext"
)

type linker interface{ TestLink(t *gotest.T) }

type ExtTestSuite struct{ l linker }

func (s *ExtTestSuite) TestByValue(t *gotest.T) {
	h := &p.ValueHarness{}
	f := h.TestValue
	f(t)
}

func (s *ExtTestSuite) TestByExpression(t *gotest.T) {
	f := (*p.ExprHarness).TestExpr
	f(&p.ExprHarness{}, t)
}

func (s *ExtTestSuite) TestByInterface(t *gotest.T) {
	if s.l != nil {
		f := s.l.TestLink
		f(t)
	}
}

func (s *ExtTestSuite) TestByCall(t *gotest.T) { (&p.CallHarness{}).TestCall(t) }

type RunnerTestSuite = p.Runner

type IntContractTestSuite = p.ContractTestSuite[int]

// ExtHelper is declared here and reached from nowhere.
type ExtHelper struct{}

func (h *ExtHelper) TestNobody(t *gotest.T) {}
