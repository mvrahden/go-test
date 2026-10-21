// Package snapkeys_test matches snapshots under the names testing gives
// rows and twins; its baselines are committed beside it.
package snapkeys_test

import "github.com/mvrahden/go-test/pkg/gotest"

type SnapKeysTestSuite struct{}

func (s *SnapKeysTestSuite) TestRows(t *gotest.T) {
	for it, v := range gotest.Each(t, []string{"row zero", "row one"}) {
		gotest.MatchSnapshot(it, v)
	}
}

func (s *SnapKeysTestSuite) TestTwins(t *gotest.T) {
	t.It("x", func(it *gotest.T) { gotest.MatchSnapshot(it, "first twin") })
	t.It("x", func(it *gotest.T) { gotest.MatchSnapshot(it, "second twin") })
}

// TestStale never matches its baseline: the run must fail it however the
// update switch is spelled when it is off.
func (s *SnapKeysTestSuite) TestStale(t *gotest.T) { gotest.MatchSnapshot(t, "new") }

// TestCarriageReturns: CRs before a newline and at the end are line endings,
// so the value matches a baseline written with plain newlines.
func (s *SnapKeysTestSuite) TestCarriageReturns(t *gotest.T) { gotest.MatchSnapshot(t, "a\r\r\nb\r") }
