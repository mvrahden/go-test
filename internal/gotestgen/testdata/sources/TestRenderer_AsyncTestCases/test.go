package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type AsyncTestSuite struct{}

func (s *AsyncTestSuite) TestPingAsync(t *gotest.T, done func()) {
	go done()
}
func (s *AsyncTestSuite) TestPlain(t *gotest.T) {}
