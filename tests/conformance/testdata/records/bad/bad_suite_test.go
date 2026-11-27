package bad_test

import "github.com/mvrahden/go-test/pkg/gotest"

type BadTestSuite struct{}

func (s *BadTestSuite) TestFails(t *gotest.T) { gotest.Equal(t, 1, 2) }

func (s *BadTestSuite) TestAlsoFails(t *gotest.T) { gotest.Equal(t, 2, 1) }
