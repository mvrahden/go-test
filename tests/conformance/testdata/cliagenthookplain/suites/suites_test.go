package suites_test

import "github.com/mvrahden/go-test/pkg/gotest"

// PlainTestSuite reads like a suite in a module that does not require
// gotest; it is never built.
type PlainTestSuite struct{}

func (s *PlainTestSuite) TestAdds(t *gotest.T) {
	gotest.True(t, 1+1 == 2)
}
