package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type fuzzCtx struct{ val string }

type ReturningFuzzTestSuite struct{}

func (s *ReturningFuzzTestSuite) BeforeEach(t *gotest.T) *fuzzCtx   { return &fuzzCtx{} }
func (s *ReturningFuzzTestSuite) TestOne(t *gotest.T, ctx *fuzzCtx) {}
func (s *ReturningFuzzTestSuite) FuzzParse(f *gotest.F) {
	gotest.Fuzz(f, func(t *gotest.T, in string) {})
}
