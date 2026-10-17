package snapshotting_test

import "github.com/mvrahden/go-test/pkg/gotest"

// SnapshottingTestSuite holds two baselines in testdata/__snapshots__: one
// the test still matches, one it no longer does. The second must fail; a
// MatchSnapshot that stops comparing would pass both.
type SnapshottingTestSuite struct{}

func (s *SnapshottingTestSuite) TestMatchesBaseline(t *gotest.T) { gotest.MatchSnapshot(t, "stable") }
func (s *SnapshottingTestSuite) TestDiffersFromBaseline(t *gotest.T) {
	gotest.MatchSnapshot(t, "changed")
}
