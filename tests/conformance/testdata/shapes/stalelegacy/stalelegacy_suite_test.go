package stalelegacy

import "github.com/mvrahden/go-test/pkg/gotest"

// NewTestSuite is what OldTestSuite was renamed to; the generated file an
// older gotest left on disk still names the old one.
type NewTestSuite struct{}

func (s *NewTestSuite) TestRuns(t *gotest.T) { gotest.True(t, true) }
