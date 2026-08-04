package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type LazySharedFixture struct {
	ConnStr string
}

type LazyTestSuite struct {
	Lazy *LazySharedFixture
}

func (s *LazyTestSuite) TestOne(t *gotest.T) {}
