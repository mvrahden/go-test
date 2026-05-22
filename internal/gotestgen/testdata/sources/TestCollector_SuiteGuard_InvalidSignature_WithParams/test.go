package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type BadTestSuite struct{}

func (s *BadTestSuite) SuiteGuard(x int) string { return "" }
func (s *BadTestSuite) TestFoo(t *gotest.T) {}
