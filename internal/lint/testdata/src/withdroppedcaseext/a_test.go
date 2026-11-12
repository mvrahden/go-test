package withdroppedcaseext

import "github.com/mvrahden/go-test/pkg/gotest"

// The analyzer sees one test package of this directory at a time, so it
// reports nothing here: the generator, which sees both, judges these.

// Probe's case is called only by the external test package.
type Probe struct{}

func (p *Probe) TestProbe(t *gotest.T) {}

// ExtContractTestSuite is instantiated only by the external test package.
type ExtContractTestSuite[T any] struct{}

func (s *ExtContractTestSuite[T]) TestHolds(t *gotest.T) {}

// Lonely is reached from nowhere; under one test package that is unproven.
type Lonely struct{}

func (l *Lonely) TestAlone(t *gotest.T) {}

type InTestSuite struct{}

func (s *InTestSuite) TestX(t *gotest.T) {}
