package listed

import "github.com/mvrahden/go-test/pkg/gotest"

// ListedTestSuite declares a focused, an excluded and a plain method: a run
// selects the focused one alone; discover lists all three.
type ListedTestSuite struct{}

func (s *ListedTestSuite) F_TestFocused(t *gotest.T) {}

func (s *ListedTestSuite) X_TestParked(t *gotest.T) { gotest.Fail(t, "an excluded case ran") }

func (s *ListedTestSuite) TestPlain(t *gotest.T) {}
