package calc_test

import "github.com/mvrahden/go-test/pkg/gotest"

type CalcTestSuite struct{}

func (s *CalcTestSuite) TestAdds(t *gotest.T) { gotest.Equal(t, 3, 1+2) }
