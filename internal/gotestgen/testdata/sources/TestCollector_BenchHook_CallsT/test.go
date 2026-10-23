package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

// TimedTestSuite's hook reaches for *testing.T, which a benchmark wrapper
// cannot hand it.
type TimedTestSuite struct{}

func (s *TimedTestSuite) BeforeEach(t *gotest.T) {
	t.T().Log("starting") // CALL:T
}

func (s *TimedTestSuite) TestOne(t *gotest.T) {}

func (s *TimedTestSuite) BenchmarkOne(b *gotest.B) {
	for b.Loop() {
	}
}
