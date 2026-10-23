package fzonly

import "github.com/mvrahden/go-test/pkg/gotest"

// SeedsTestSuite marks each seed it replays.
type SeedsTestSuite struct {
	Shared *MarkSharedFixture
}

func (s *SeedsTestSuite) FuzzOne(f *gotest.F) {
	f.Add("one")
	f.Fuzz(func(t *gotest.T, in string) { mark("seed ran") })
}

func (s *SeedsTestSuite) FuzzTwo(f *gotest.F) {
	f.Add("two")
	f.Fuzz(func(t *gotest.T, in string) { mark("seed ran") })
}
