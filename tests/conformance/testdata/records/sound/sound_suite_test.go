package sound_test

import "github.com/mvrahden/go-test/pkg/gotest"

type SoundTestSuite struct{}

func (s *SoundTestSuite) TestHolds(t *gotest.T) { gotest.Equal(t, 1, 1) }
