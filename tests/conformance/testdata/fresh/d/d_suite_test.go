package d_test

import "github.com/mvrahden/go-test/pkg/gotest"

type DTestSuite struct{}

func (s *DTestSuite) TestPasses(t *gotest.T) { gotest.True(t, true) }

func (s *DTestSuite) TestFails(t *gotest.T) { gotest.True(t, false, "fails on purpose") }
