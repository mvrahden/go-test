package lintdroppedgated

import "github.com/mvrahden/go-test/pkg/gotest"

// gated's case is called only from gated_test.go, which the build leaves out.
type gated struct{}

func (g gated) TestGated(t *gotest.T) {}

// GatedContractTestSuite is instantiated only by gated_test.go.
type GatedContractTestSuite[T any] struct{}

func (s *GatedContractTestSuite[T]) TestHolds(t *gotest.T) {}

// probe is handed to an interface that holds its case.
type probe struct{}

func (p *probe) TestProbes(t *gotest.T) {}

type prober interface{ TestProbes(t *gotest.T) }

var _ prober = (*probe)(nil)

// Lonely is reached from nowhere.
type Lonely struct{}

func (l *Lonely) TestAlone(t *gotest.T) {}

type OpenTestSuite struct{}

func (s *OpenTestSuite) TestOpens(t *gotest.T) {}
