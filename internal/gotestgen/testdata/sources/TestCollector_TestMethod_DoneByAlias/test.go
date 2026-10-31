package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type doneFn = func()

type AliasDoneTestSuite struct{}

func (s *AliasDoneTestSuite) TestPingAsync(t *gotest.T, d doneFn) { d() }
