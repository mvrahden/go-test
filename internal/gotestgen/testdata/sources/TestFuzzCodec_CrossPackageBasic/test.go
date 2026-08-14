package testpkg

import (
	"github.com/mvrahden/go-test/pkg/gotest"

	"testpkg/TestFuzzCodec_CrossDep"
)

// CrossPkgBasicFuzzTestSuite fuzzes crossdep.ID directly — a cross-package
// named basic with no enclosing struct — so its literal function is built
// by literalHelper as a top-level entry point, and its wrap must be the
// QUALIFIED crossdep.ID(...), not the bare (out-of-scope) ID(...).
type CrossPkgBasicFuzzTestSuite struct{}

func (s *CrossPkgBasicFuzzTestSuite) TestOne(t *gotest.T) {}

func (s *CrossPkgBasicFuzzTestSuite) FuzzTag(f *gotest.F) {
	gotest.Fuzz(f, func(t *gotest.T, tag crossdep.ID) { _ = tag })
}
