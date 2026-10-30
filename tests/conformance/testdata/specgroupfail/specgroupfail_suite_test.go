// Package specgroupfail declares a When whose body fails after the only It
// under it passed.
package specgroupfail

import "github.com/mvrahden/go-test/pkg/gotest"

type BootTestSuite struct{}

func (s *BootTestSuite) TestStart(t *gotest.T) {
	t.When("the disk | is full", func(w *gotest.T) {
		w.It("starts", func(it *gotest.T) {})
		gotest.Fail(w, "no space <left>")
	})
}
