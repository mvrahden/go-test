//go:build windows

package gotestrunner_test

import (
	"golang.org/x/sys/windows"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// RelayConsoleTestSuite: on Windows the converter runs with no console, so no
// console control event sent to the suite's console reaches it.
type RelayConsoleTestSuite struct{}

func (s *RelayConsoleTestSuite) TestTheConverterStartsApart(t *gotest.T) {
	cmd := gotestrunner.ExportConverterCmd(t.Context(), "example.com/p", "")
	gotest.NotZero(t, cmd.SysProcAttr)
	t.When("a converter is made", func(w *gotest.T) {
		w.It("starts detached from every console", func(it *gotest.T) {
			gotest.Equal(it, uint32(windows.DETACHED_PROCESS), cmd.SysProcAttr.CreationFlags&windows.DETACHED_PROCESS)
		})
	})
}
