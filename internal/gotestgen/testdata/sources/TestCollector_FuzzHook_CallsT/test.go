package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

// SeededTestSuite's hook reaches for *testing.T, which a fuzz wrapper cannot
// hand it.
type SeededTestSuite struct{}

func (s *SeededTestSuite) AfterAll(t *gotest.T) {
	t.T().Log("done") // CALL:T
}

func (s *SeededTestSuite) TestOne(t *gotest.T) {}

func (s *SeededTestSuite) FuzzOne(f *gotest.F) {
	f.Add("seed")
	f.Fuzz(func(t *gotest.T, in string) {})
}
