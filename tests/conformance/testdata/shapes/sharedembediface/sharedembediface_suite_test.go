package sharedembediface

import "github.com/mvrahden/go-test/pkg/gotest"

type ClockTestSuite struct{ Clock *ClockSharedFixture }

func (s *ClockTestSuite) TestNow(t *gotest.T) { gotest.NotZero(t, s.Clock.Now()) }
