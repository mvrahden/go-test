package namebarehelper

import "github.com/mvrahden/go-test/pkg/gotest"

// HelperNamesTestSuite has helpers named by a case prefix alone that take no T, B or F.
type HelperNamesTestSuite struct{}

func (s *HelperNamesTestSuite) Test() bool { return true }

func (s *HelperNamesTestSuite) Benchmark(n int) int { return n }

func (s HelperNamesTestSuite) Fuzz(seed string) string { return seed }

func (s *HelperNamesTestSuite) TestRuns(t *gotest.T) {
	gotest.True(t, s.Test())
	gotest.Equal(t, 1, s.Benchmark(1))
	gotest.Equal(t, "a", s.Fuzz("a"))
}
