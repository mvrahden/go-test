package e2eexcluded

import "github.com/mvrahden/go-test/pkg/gotest"

// X_RetiredTestSuite is excluded: none of its methods may run.
type X_RetiredTestSuite struct{}

func (s *X_RetiredTestSuite) TestRetired(t *gotest.T) { gotest.Fail(t, "an excluded suite ran") }

type ActiveTestSuite struct{}

func (s *ActiveTestSuite) TestRuns(t *gotest.T) { gotest.True(t, true) }
