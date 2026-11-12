//go:build lintdroppedgated

package lintdroppedgated

import "github.com/mvrahden/go-test/pkg/gotest"

type IntGatedTestSuite = GatedContractTestSuite[int]

type GatedTestSuite struct{ g gated }

func (s *GatedTestSuite) TestRuns(t *gotest.T) { s.g.TestGated(t) }
