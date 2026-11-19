package stalebroken

import "github.com/mvrahden/go-test/pkg/gotest"

// NewTestSuite does not compile; the generated file on disk still names the
// suite it was renamed from.
type NewTestSuite struct{}

func (s *NewTestSuite) TestRuns(t *gotest.T) {
	var n int = "not an int"
	gotest.Zero(t, n)
}
