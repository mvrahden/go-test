package sharediface

import "github.com/mvrahden/go-test/pkg/gotest"

type ClockTestSuite struct{ Clock *ClockSharedFixture }

func (s *ClockTestSuite) TestBody(t *gotest.T) {}
