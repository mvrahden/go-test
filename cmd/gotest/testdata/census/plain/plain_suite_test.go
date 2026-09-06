package plain_test

import "github.com/mvrahden/go-test/pkg/gotest"

type PlainTestSuite struct{}

func (s *PlainTestSuite) TestOne(t *gotest.T) { gotest.True(t, true) }
func (s *PlainTestSuite) TestTwo(t *gotest.T) { gotest.True(t, true) }
