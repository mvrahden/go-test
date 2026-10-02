//go:build unix

package gotestrunner_test

import (
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// BinaryCopyExecTestSuite covers the step between the binary cache and a
// suite process: the run copies a binary into its work dir and executes the
// copy at once, while it keeps starting other processes.
type BinaryCopyExecTestSuite struct{}

// A process forked while the copy is open for writing inherits that handle
// until it execs, and the kernel refuses to execute a file anyone holds open
// for writing ("text file busy").
func (s *BinaryCopyExecTestSuite) TestACopyStartsWhileTheProcessForks(t *gotest.T) {
	// The converter is a few megabytes and exits at once on an empty stdin.
	src := gotestrunner.ExportTest2JSONPath()
	gotest.NotEmpty(t, src)
	forked, err := exec.LookPath("true")
	gotest.NoError(t, err)

	for range 8 {
		gotest.Go(t, func() {
			for t.Context().Err() == nil {
				_ = exec.Command(forked).Run()
			}
		})
	}

	dir := t.TempDir()
	for i := range 100 {
		dst := filepath.Join(dir, fmt.Sprintf("copy-%d", i))
		gotest.NoError(t, gotestrunner.ExportCopyFile(src, dst))
		out, err := exec.Command(dst).CombinedOutput() //nolint:gosec // G204: a copy of the Go toolchain's converter
		gotest.NoError(t, err, "copy %d did not run: %s", i, out)
	}
}
