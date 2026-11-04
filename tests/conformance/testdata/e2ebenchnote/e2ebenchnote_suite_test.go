package e2ebenchnote

import "github.com/mvrahden/go-test/pkg/gotest"

// NotedTestSuite declares a benchmark beside its test: a test run leaves it out.
type NotedTestSuite struct{}

func (s *NotedTestSuite) TestRuns(t *gotest.T) { gotest.True(t, true) }

func (s *NotedTestSuite) BenchmarkLeftOut(b *gotest.B) {
	for b.Loop() {
	}
}
