package linkname_test

import (
	_ "unsafe"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// traceback1 reaches into the runtime with a linkname the linker refuses
// since Go 1.23; go test fails the package at link time, and so must gotest.
//
//go:linkname traceback1 runtime.traceback1
func traceback1()

type LinknameTestSuite struct{}

func (s *LinknameTestSuite) TestNeverLinks(t *gotest.T) {
	traceback1()
	gotest.True(t, true)
}
