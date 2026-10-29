package a

import "github.com/mvrahden/go-test/pkg/gotest"

// SeedsTestSuite is unfocused and holds the run's only focused method, a fuzz target.
type SeedsTestSuite struct{}

func (s *SeedsTestSuite) TestPlain(t *gotest.T) { gotest.Fail(t, "an unfocused method ran") }

func (s *SeedsTestSuite) F_FuzzSeeds(f *gotest.F) {
	f.Add(1)
	f.Fuzz(func(t *gotest.T, n int) {})
}
