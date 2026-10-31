package lintfocusspread

import "github.com/mvrahden/go-test/pkg/gotest"

type F_SpreadTestSuite struct{ n int }

func (s *F_SpreadTestSuite) BeforeEach(t *gotest.T) { s.n = 1 }

func (s *F_SpreadTestSuite) TestOne(t *gotest.T) { gotest.Equal(t, 1, s.n) }
