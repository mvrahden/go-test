package toggleembedded

import "github.com/mvrahden/go-test/pkg/gotest"

type BaseTestSuite struct{ n int }

func (s *BaseTestSuite) TestBase(t *gotest.T) { gotest.Equal(t, 0, s.n) }
