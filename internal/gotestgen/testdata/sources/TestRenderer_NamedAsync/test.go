package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type AsyncTestSuite struct{}

func (s *AsyncTestSuite) TestPingAsync(t *gotest.T) {}
func (s *AsyncTestSuite) TestPlain(t *gotest.T)     {}
