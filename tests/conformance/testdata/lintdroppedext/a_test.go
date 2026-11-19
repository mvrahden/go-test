package lintdroppedext

import "github.com/mvrahden/go-test/pkg/gotest"

// The external test package reaches each of these cases; nothing here does.
type ValueHarness struct{}

func (h *ValueHarness) TestValue(t *gotest.T) {}

type ExprHarness struct{}

func (h *ExprHarness) TestExpr(t *gotest.T) {}

type LinkHarness struct{}

func (h *LinkHarness) TestLink(t *gotest.T) {}

type CallHarness struct{}

func (h *CallHarness) TestCall(t *gotest.T) {}

// Runner runs as RunnerTestSuite, an alias the external package declares.
type Runner struct{}

func (r *Runner) TestRuns(t *gotest.T) {}

// ContractTestSuite runs as IntContractTestSuite, aliased there too.
type ContractTestSuite[T any] struct{}

func (s *ContractTestSuite[T]) TestHolds(t *gotest.T) {}

// Lonely is reached from nowhere.
type Lonely struct{}

func (l *Lonely) TestAlone(t *gotest.T) {}

type InTestSuite struct{}

func (s *InTestSuite) TestX(t *gotest.T) {}
