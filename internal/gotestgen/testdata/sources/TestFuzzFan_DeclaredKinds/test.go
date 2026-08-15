package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type DeclaredFuzzTestSuite struct{}

func (s *DeclaredFuzzTestSuite) TestOne(t *gotest.T) {}

// FuzzInt declares a plain number: it fans, so the engine mutates it as a
// fixed-width []byte leaf rather than by ±100 arithmetic.
func (s *DeclaredFuzzTestSuite) FuzzInt(f *gotest.F) {
	f.Add(7)
	gotest.Fuzz(f, func(t *gotest.T, n int) {
		gotest.True(t, n == n)
	})
}

// FuzzTwoStrings is all pass-through: no fan, the engine sees it as declared.
func (s *DeclaredFuzzTestSuite) FuzzTwoStrings(f *gotest.F) {
	f.Add("a", "b")
	gotest.Fuzz2(f, func(t *gotest.T, a, b string) {
		gotest.True(t, a == a && b == b)
	})
}

// FuzzMixed3 mixes a pass-through, a number, and a []byte.
func (s *DeclaredFuzzTestSuite) FuzzMixed3(f *gotest.F) {
	f.Add("a", uint16(1), []byte("x"))
	gotest.Fuzz3(f, func(t *gotest.T, a string, n uint16, b []byte) {
		gotest.True(t, a == a && n == n && len(b) >= 0)
	})
}
