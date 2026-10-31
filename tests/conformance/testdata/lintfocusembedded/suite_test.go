package lintfocusembedded

import "github.com/mvrahden/go-test/pkg/gotest"

type F_BaseTestSuite struct{ n int }

func (s *F_BaseTestSuite) TestBase(t *gotest.T) { gotest.Equal(t, 0, s.n) }
