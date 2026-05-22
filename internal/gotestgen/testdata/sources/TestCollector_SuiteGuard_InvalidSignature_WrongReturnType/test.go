package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type BadTestSuite struct{}

func (s *BadTestSuite) SuiteGuard() int { return 0 }
func (s *BadTestSuite) TestFoo(t *gotest.T) {}
