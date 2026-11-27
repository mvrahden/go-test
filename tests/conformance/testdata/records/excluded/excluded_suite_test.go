package excluded_test

import "github.com/mvrahden/go-test/pkg/gotest"

type ExcludedTestSuite struct{}

func (s *ExcludedTestSuite) X_TestOff(t *gotest.T) { gotest.Equal(t, 1, 2) }
