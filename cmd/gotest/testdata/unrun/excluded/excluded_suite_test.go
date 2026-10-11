package excluded_test

import "github.com/mvrahden/go-test/pkg/gotest"

// X_ExcludedTestSuite is excluded: the package has a test file and runs
// nothing.
type X_ExcludedTestSuite struct{}

func (s *X_ExcludedTestSuite) TestA(t *gotest.T) {}
