package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type Pair struct {
	Left  int
	Right int
}

type MultiArgFuzzTestSuite struct{}

func (s *MultiArgFuzzTestSuite) TestOne(t *gotest.T) {}

func (s *MultiArgFuzzTestSuite) FuzzPair(f *gotest.F) {
	gotest.Fuzz2(f, func(t *gotest.T, p Pair, n int) {
		gotest.True(t, p.Left == p.Left && n == n)
	})
}
