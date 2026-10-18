package aliasrecv

import "github.com/mvrahden/go-test/pkg/gotest"

// RealTestSuite gets its method through an alias of itself.
type RealTestSuite struct{ n int }

// Real is an alias, not a suite of its own.
type Real = RealTestSuite

func (s *Real) TestViaAlias(t *gotest.T) { gotest.Zero(t, s.n) }
