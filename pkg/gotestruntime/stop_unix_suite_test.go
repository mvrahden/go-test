//go:build unix

package gotestruntime_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/pkg/gotestruntime"
)

// A run of this test binary with envPublishChild set to a stop file is the
// watched process of the scenario below.
const envPublishChild = "GOTESTRUNTIME_PUBLISH_CHILD"

// PublishTestSuite covers the pid a watched process publishes for the
// runner, which signals it at once.
type PublishTestSuite struct{}

func (s *PublishTestSuite) TestThePidIsPublishedOnceTheSignalsAreCaught(t *gotest.T) {
	if stop := os.Getenv(envPublishChild); stop != "" {
		// SIGTERM the moment the pid is out, as a runner reading it would.
		restore := gotestruntime.ExportAfterPublishPID(func() {
			_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
			time.Sleep(200 * time.Millisecond)
		})
		unwatch := gotestruntime.ExportWatchStopFile(t.T(), stop)
		unwatch()
		restore()
		return
	}
	dir := t.TempDir()
	stop := filepath.Join(dir, "stop")
	cmd := exec.Command(os.Args[0], "-test.run=^TestPublishTestSuite$/^TestThePidIsPublishedOnceTheSignalsAreCaught$") //nolint:gosec // G204: this test binary
	cmd.Env = append(os.Environ(), envPublishChild+"="+stop)
	out, err := cmd.CombinedOutput()
	data, _ := os.ReadFile(protocol.StopPIDPath(stop))
	entries, _ := os.ReadDir(dir)

	t.When("a watched process is signalled the moment its pid is published", func(w *gotest.T) {
		w.It("has caught the signal by then", func(it *gotest.T) {
			gotest.NoError(it, err, string(out))
		})
		w.It("leaves the whole pid in the file, and nothing else", func(it *gotest.T) {
			gotest.Equal(it, strconv.Itoa(cmd.Process.Pid), string(data))
			gotest.Len(it, entries, 1)
		})
	})
}
