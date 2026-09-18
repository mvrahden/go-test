package fuzzing_test

import (
	"strings"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// FuzzingTestSuite declares two fuzz targets whose seeds replay on every
// run: one holds, one fails on its seed. A run must report both.
type FuzzingTestSuite struct{}

func (s *FuzzingTestSuite) TestSeedsReplay(t *gotest.T) { gotest.True(t, true) }

func (s *FuzzingTestSuite) FuzzHolds(f *gotest.F) {
	f.Add(" a ")
	f.Fuzz(func(t *gotest.T, in string) {
		once := strings.TrimSpace(in)
		gotest.Equal(t, once, strings.TrimSpace(once))
	})
}

func (s *FuzzingTestSuite) FuzzBroken(f *gotest.F) {
	f.Add("seed")
	f.Fuzz(func(t *gotest.T, in string) {
		gotest.Equal(t, "", in)
	})
}
