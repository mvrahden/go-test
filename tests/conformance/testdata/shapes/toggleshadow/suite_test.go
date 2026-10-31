package toggleshadow

import "github.com/mvrahden/go-test/pkg/gotest"

type ShadowTestSuite struct{}

func (s *ShadowTestSuite) TestOne(t *gotest.T) { gotest.Equal(t, 1, local()) }
