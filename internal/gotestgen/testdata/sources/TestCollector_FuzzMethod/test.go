package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type FuzzTestSuite struct{}

func (s *FuzzTestSuite) BeforeEach(t *gotest.T) {}
func (s *FuzzTestSuite) TestOne(t *gotest.T)    {}
func (s *FuzzTestSuite) FuzzParse(f *gotest.F) {
	gotest.Fuzz(f, func(t *gotest.T, in string) {})
}
func (s *FuzzTestSuite) X_FuzzOld(f *gotest.F) {
	gotest.Fuzz(f, func(t *gotest.T, in string) {})
}
