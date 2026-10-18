package focused_test

import "github.com/mvrahden/go-test/pkg/gotest"

// FocusedTestSuite has a focused method, so a run of it is green unless the
// focus guard is on, which turns a focused run into a failure.
type FocusedTestSuite struct{}

func (s *FocusedTestSuite) F_TestOne(t *gotest.T) { gotest.True(t, true) }
func (s *FocusedTestSuite) TestTwo(t *gotest.T)   { gotest.True(t, true) }
