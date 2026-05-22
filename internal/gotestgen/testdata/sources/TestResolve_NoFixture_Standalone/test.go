package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type PlainTestSuite struct{ val string }
func (s *PlainTestSuite) TestOne(t *gotest.T) {}
