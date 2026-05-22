package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type MyTestSuite struct{}

func (s *MyTestSuite) SuiteGuard() string { return "" }
func (s *MyTestSuite) TestFoo(t *gotest.T) {}
