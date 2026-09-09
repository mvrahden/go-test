package fuzzcalls

import "github.com/mvrahden/go-test/pkg/gotest"

type Header struct{ Name, Value string }

// OkTestSuite is the well-formed shape: one f.Fuzz call with a literal.
type OkTestSuite struct{}

func (s *OkTestSuite) FuzzPair(f *gotest.F) {
	f.Add(Header{Name: "n", Value: "v"}, "topic")
	f.Fuzz(func(t *gotest.T, h Header, topic string) {})
}

// NamedTestSuite binds a method value instead of a literal.
type NamedTestSuite struct{}

func (s *NamedTestSuite) FuzzNamed(f *gotest.F) { f.Fuzz(s.check) }

func (s *NamedTestSuite) check(t *gotest.T, n int) {}

// TwiceTestSuite calls f.Fuzz twice; the engine allows one target per F.
type TwiceTestSuite struct{}

func (s *TwiceTestSuite) FuzzTwice(f *gotest.F) {
	f.Fuzz(func(t *gotest.T, s string) {})
	f.Fuzz(func(t *gotest.T, s string) {})
}

// NoneTestSuite never binds a callback.
type NoneTestSuite struct{}

func (s *NoneTestSuite) FuzzNone(f *gotest.F) { f.Add("x") }

// NotFuncTestSuite hands f.Fuzz something that is not a function.
type NotFuncTestSuite struct{}

func (s *NotFuncTestSuite) FuzzNotFunc(f *gotest.F) { f.Fuzz("not a func") }

// NoTTestSuite's callback lacks the leading *gotest.T.
type NoTTestSuite struct{}

func (s *NoTTestSuite) FuzzNoT(f *gotest.F) { f.Fuzz(func(s string) {}) }
