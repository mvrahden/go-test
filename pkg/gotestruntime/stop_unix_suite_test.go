//go:build unix

package gotestruntime_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
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

func (s *PublishTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

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

// A run of this test binary with envSettledChild set is the watched process
// of the scenario below: its suite stops by its stop file, then TestMain
// signals it.
const envSettledChild = "GOTESTRUNTIME_SETTLED_CHILD"

// envSettledStop names the stop file the watched process creates itself.
const envSettledStop = "GOTESTRUNTIME_SETTLED_STOP"

// The runner creates its stop file, then signals; the file alone can settle
// the run first, so the signal lands on a process that is already ending.
func (s *PublishTestSuite) TestASignalAfterTheStopFileSettledTheRunIsNotFatal(t *gotest.T) {
	if os.Getenv(envSettledChild) == "1" {
		gotest.NoError(t, os.WriteFile(os.Getenv(envSettledStop), nil, 0o600))
		<-t.Context().Done() // the stop the file began ends it
		return
	}
	stop := filepath.Join(t.TempDir(), "stop")
	cmd := exec.Command(os.Args[0], "-test.run=^TestPublishTestSuite$/^TestASignalAfterTheStopFileSettledTheRunIsNotFatal$") //nolint:gosec // G204: this test binary
	cmd.Env = append(os.Environ(), envSettledChild+"=1", envSettledStop+"="+stop, protocol.EnvStopFile+"="+stop)
	out, err := cmd.CombinedOutput()

	t.When("the runner's signal reaches a process whose stop file already settled its run", func(w *gotest.T) {
		w.It("is not ended by the signal", func(it *gotest.T) {
			gotest.NoError(it, err, string(out))
		})
	})
}

// TestMain, in the watched process, sends it the runner's signal once its
// run has ended, as a TestMain releasing what it set up would still be
// running then.
func TestMain(m *testing.M) {
	code := m.Run()
	if os.Getenv(envSettledChild) == "1" {
		time.Sleep(500 * time.Millisecond)
		if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
			panic(err)
		}
		time.Sleep(500 * time.Millisecond)
	}
	os.Exit(code)
}
