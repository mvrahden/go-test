package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type NoSetupFixture struct{ Val string }

type SomeTestSuite struct { *NoSetupFixture }
func (s *SomeTestSuite) TestOne(t *gotest.T) {}
