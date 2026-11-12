package orphangoos

import "github.com/mvrahden/go-test/pkg/gotest"

type SignalTestSuite struct{ c sigChecks }

func (s *SignalTestSuite) TestSig(t *gotest.T) { s.c.TestSignals(t) }
