package focusfuzz_test

import "github.com/mvrahden/go-test/pkg/gotest"

// SeedsTestSuite holds the run's only focused method, a fuzz target.
type SeedsTestSuite struct{}

func (s *SeedsTestSuite) TestPlain(t *gotest.T) {}

func (s *SeedsTestSuite) F_FuzzSeeds(f *gotest.F) {
	f.Add(1)
	f.Fuzz(func(t *gotest.T, n int) {})
}
