package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type MyTestSuite struct{}

func (s *MyTestSuite) TestOneAsync(t *gotest.T, done chan struct{}) {}
