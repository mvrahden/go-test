package setupfailing

import "github.com/mvrahden/go-test/pkg/gotest"

// SetupFailingTestSuite never runs; the shared fixture it reads fails to set up.
type SetupFailingTestSuite struct {
	Probe *DownSharedFixture
}

func (s *SetupFailingTestSuite) TestFixtureIsUp(t *gotest.T) {
	gotest.Equal(t, "up", s.Probe.State)
}

func (s *SetupFailingTestSuite) BenchmarkRead(b *gotest.B) {
	for b.Loop() {
		_ = s.Probe.State
	}
}
