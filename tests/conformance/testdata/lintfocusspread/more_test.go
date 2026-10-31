package lintfocusspread

import "github.com/mvrahden/go-test/pkg/gotest"

func (s *F_SpreadTestSuite) TestTwo(t *gotest.T) { gotest.Equal(t, 1, s.n) }
