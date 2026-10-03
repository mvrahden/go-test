package setupfailing

import "github.com/mvrahden/go-test/pkg/gotest"

// AloneTestSuite runs after every other suite and reads a fixture of its
// own, which a failure of another fixture must leave alone.
type AloneTestSuite struct {
	Probe *UpSharedFixture
}

func (s *AloneTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Exclusive: true}
}

func (s *AloneTestSuite) TestFixtureIsUp(t *gotest.T) {
	gotest.Equal(t, "up", s.Probe.State)
}

func (s *AloneTestSuite) BenchmarkFixtureIsUp(b *gotest.B) {
	for b.Loop() {
		_ = s.Probe.State
	}
}
