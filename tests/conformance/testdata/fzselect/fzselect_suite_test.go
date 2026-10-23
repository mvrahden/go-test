package fzselect

import "github.com/mvrahden/go-test/pkg/gotest"

// MixedTestSuite marks each method it runs.
type MixedTestSuite struct {
	Shared *MarkSharedFixture
}

func (s *MixedTestSuite) TestOne(t *gotest.T) { mark("test ran") }

func (s *MixedTestSuite) FuzzOne(f *gotest.F) {
	f.Add("seed")
	f.Fuzz(func(t *gotest.T, in string) { mark("seed ran") })
}
