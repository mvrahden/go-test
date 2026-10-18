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

// TestRows and TestTwins read baselines keyed by the names testing gives
// unnamed rows and same-named subtests.
func (s *SnapshottingTestSuite) TestRows(t *gotest.T) {
	for it, v := range gotest.Each(t, []string{"row zero", "row one"}) {
		gotest.MatchSnapshot(it, v)
	}
}

func (s *SnapshottingTestSuite) TestTwins(t *gotest.T) {
	t.It("x", func(it *gotest.T) { gotest.MatchSnapshot(it, "first twin") })
	t.It("x", func(it *gotest.T) { gotest.MatchSnapshot(it, "second twin") })
}
