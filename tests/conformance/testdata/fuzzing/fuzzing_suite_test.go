package fuzzing_test

import (
	"strings"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// FuzzyTestSuite has one test and one fuzz target, so a -run filter naming
// the generated fuzz wrapper has a suite to select.
type FuzzyTestSuite struct{}

func (s *FuzzyTestSuite) TestUpper(t *gotest.T) { gotest.Equal(t, "A", strings.ToUpper("a")) }

func (s *FuzzyTestSuite) FuzzUpperIsIdempotent(f *gotest.F) {
	f.Add("seed")
	f.Fuzz(func(t *gotest.T, in string) {
		up := strings.ToUpper(in)
		gotest.Equal(t, up, strings.ToUpper(up))
	})
}
