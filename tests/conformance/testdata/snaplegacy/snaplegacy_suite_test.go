// Package snaplegacy_test reads baselines keyed the way the release before
// exact subtest names keyed them, committed beside it.
package snaplegacy_test

import "github.com/mvrahden/go-test/pkg/gotest"

type SnapLegacyTestSuite struct{}

// TestIssue: "issue #42" was keyed without testing's "#42".
func (s *SnapLegacyTestSuite) TestIssue(t *gotest.T) {
	t.It("issue #42", func(it *gotest.T) { gotest.MatchSnapshot(it, "forty-two") })
}

// TestRows: a named snapshot of an unnamed row was keyed without its "#0".
func (s *SnapLegacyTestSuite) TestRows(t *gotest.T) {
	for it, v := range gotest.Each(t, []string{"row zero"}) {
		gotest.MatchSnapshot(it, v, "n")
	}
}

// TestTwins: twins shared the section "x", which the first keeps.
func (s *SnapLegacyTestSuite) TestTwins(t *gotest.T) {
	t.It("x", func(it *gotest.T) { gotest.MatchSnapshot(it, "first twin") })
	t.It("x", func(it *gotest.T) { gotest.MatchSnapshot(it, "second twin") })
}

// TestStale: its old-keyed baseline no longer matches.
func (s *SnapLegacyTestSuite) TestStale(t *gotest.T) {
	t.It("issue #7", func(it *gotest.T) { gotest.MatchSnapshot(it, "new") })
}
