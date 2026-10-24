package noisysetup

import "github.com/mvrahden/go-test/pkg/gotest"

// NoisyTestSuite reads the state of a fixture whose setup wrote to stdout.
type NoisyTestSuite struct {
	Noise *NoiseSharedFixture
}

func (s *NoisyTestSuite) TestStateArrived(t *gotest.T) { gotest.Equal(t, "arrived", s.Noise.Note) }
