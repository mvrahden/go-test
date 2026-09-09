package fuzzing

import (
	"strings"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// FuzzingTestSuite carries one test and one fuzz target, so the extension's
// end-to-end tests see a fuzz target beside a method in the same suite.
type FuzzingTestSuite struct{}

func (s *FuzzingTestSuite) TestTrim(t *gotest.T) {
	t.It("drops surrounding spaces", func(it *gotest.T) {
		gotest.Equal(it, "x", strings.TrimSpace("  x "))
	})
}

func (s *FuzzingTestSuite) FuzzTrimIdempotent(f *gotest.F) {
	f.Add(" a ")
	f.Add("b")
	f.Fuzz(func(t *gotest.T, in string) {
		once := strings.TrimSpace(in)
		gotest.Equal(t, once, strings.TrimSpace(once))
	})
}
