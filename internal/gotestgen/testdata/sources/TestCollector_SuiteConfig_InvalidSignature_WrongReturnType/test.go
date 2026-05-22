package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type BadTestSuite struct{}

func (s *BadTestSuite) SuiteConfig() int { return 0 }
func (s *BadTestSuite) TestFoo(t *gotest.T) {}
