package half

import "github.com/mvrahden/go-test/pkg/gotest"

type HalfTestSuite struct{}

func (s *HalfTestSuite) TestAdd(t *gotest.T) { gotest.Equal(t, 3, Add(1, 2)) }
