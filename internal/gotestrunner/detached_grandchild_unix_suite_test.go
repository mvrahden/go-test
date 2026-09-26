//go:build !windows

package gotestrunner_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// DetachedGrandchildTestSuite runs a suite process whose grandchild leaves the
// process group and keeps stdout open, the shape a daemonizing helper takes.
// Exclusive: it takes a wall-clock verdict on how long Wait blocks.
type DetachedGrandchildTestSuite struct {
	stopFile string
}

func (s *DetachedGrandchildTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Exclusive = true
	return cfg
}

func (s *DetachedGrandchildTestSuite) BeforeEach(t *gotest.T) {
	s.stopFile = filepath.Join(t.TempDir(), "stop")
}

// AfterEach releases the grandchild, which polls for the stop file.
func (s *DetachedGrandchildTestSuite) AfterEach(t *gotest.T) {
	_ = os.WriteFile(s.stopFile, []byte("stop"), 0o600)
}

// playDetachRole is the re-executed test binary's part: the child writes a
// line, starts a session-leader grandchild on the inherited stdout and exits;
// the grandchild holds that stdout until the stop file appears. The test
// process itself returns at once.
func playDetachRole(runArg string) {
	switch os.Getenv("GOTEST_MP_DETACH") {
	case "child":
		fmt.Println("child ran")
		grandchild := exec.Command(os.Args[0], runArg) //nolint:gosec // G204: test-only subprocess
		grandchild.Env = append(os.Environ(), "GOTEST_MP_DETACH=grandchild")
		grandchild.Stdout = os.Stdout
		grandchild.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err := grandchild.Start(); err != nil {
			os.Exit(3)
		}
		os.Exit(0)
	case "grandchild":
		stop := os.Getenv("GOTEST_MP_STOP_FILE")
		for range 400 {
			if _, err := os.Stat(stop); err == nil {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		os.Exit(0)
	}
}

// detachingCommand is the child process with a buffer for stdout, as
// RunSingleSuite captures it.
func (s *DetachedGrandchildTestSuite) detachingCommand(ctx context.Context, runArg string) (*exec.Cmd, *bytes.Buffer) {
	cmd := exec.CommandContext(ctx, os.Args[0], runArg) //nolint:gosec // G204: test-only subprocess
	cmd.Env = append(os.Environ(), "GOTEST_MP_DETACH=child", "GOTEST_MP_STOP_FILE="+s.stopFile)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	return cmd, &stdout
}

// The child is gone at once and only the grandchild holds its stdout: the
// wait ends when the output drain delay passes, with the child's output kept,
// under the default configuration.
func (s *DetachedGrandchildTestSuite) TestWaitReturnsOnceTheOutputIsDrained(t *gotest.T) {
	const runArg = "-test.run=^TestDetachedGrandchildTestSuite$/^TestWaitReturnsOnceTheOutputIsDrained$"
	playDetachRole(runArg)

	cmd, stdout := s.detachingCommand(context.Background(), runArg)
	mp := gotestrunner.NewManagedProcess(cmd, gotestrunner.ProcessConfig{
		Grace:         gotestrunner.GraceFixed,
		GraceDuration: 100 * time.Millisecond,
	})
	gotest.NoError(t, mp.Start())

	start := time.Now()
	_ = mp.WaitWithGrace(context.Background())
	elapsed := time.Since(start)

	t.It("returns once the drain delay passes, not when the grandchild lets go", func(it *gotest.T) {
		gotest.Less(it, elapsed, gotestrunner.OutputDrainDelay+3*time.Second, "Wait blocked %v on a pipe a detached grandchild holds", elapsed)
	})
	t.It("keeps what the child wrote", func(it *gotest.T) {
		gotest.Contains(it, stdout.String(), "child ran")
	})
}

// After a shutdown request the same bound holds, from the moment the tree is
// gone.
func (s *DetachedGrandchildTestSuite) TestWaitReturnsWithinTheDrainDelayAfterShutdown(t *gotest.T) {
	const runArg = "-test.run=^TestDetachedGrandchildTestSuite$/^TestWaitReturnsWithinTheDrainDelayAfterShutdown$"
	playDetachRole(runArg)

	ctx, cancel := context.WithCancel(context.Background())
	cmd, _ := s.detachingCommand(ctx, runArg)
	mp := gotestrunner.NewManagedProcess(cmd, gotestrunner.ProcessConfig{
		Grace:         gotestrunner.GraceFixed,
		GraceDuration: 100 * time.Millisecond,
		DrainDelay:    time.Second,
	})
	gotest.NoError(t, mp.Start())
	// The child is gone at once; the grandchild still holds its stdout.
	time.Sleep(300 * time.Millisecond)
	cancel()

	start := time.Now()
	_ = mp.WaitWithGrace(ctx)
	elapsed := time.Since(start)

	t.It("returns once the drain delay passes, not when the grandchild lets go", func(it *gotest.T) {
		gotest.Less(it, elapsed, 5*time.Second, "Wait blocked %v on a pipe a detached grandchild holds", elapsed)
	})
}
