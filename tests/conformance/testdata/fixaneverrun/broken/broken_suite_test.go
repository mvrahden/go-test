package broken_test

import (
	_ "unsafe"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/conformance/testdata/fixaneverrun/fixtures"
)

// traceback1 reaches into the runtime with a linkname the linker refuses, so
// this package is planned but never runs.
//
//go:linkname traceback1 runtime.traceback1
func traceback1()

// BrokenTestSuite reads the fixture too.
type BrokenTestSuite struct {
	A *fixtures.ASharedFixture
}

func (s *BrokenTestSuite) TestNeverLinks(t *gotest.T) {
	traceback1()
	gotest.Equal(t, "up", s.A.State)
}
