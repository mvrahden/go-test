package fzsharedfail

import "github.com/mvrahden/go-test/pkg/gotest"

// DownTestSuite binds the fixture that never comes up.
type DownTestSuite struct {
	Down *DownSharedFixture
}

// FuzzRead marks that its body ran.
func (s *DownTestSuite) FuzzRead(f *gotest.F) {
	f.Add("seed")
	f.Fuzz(func(t *gotest.T, in string) { mark("body ran") })
}
