//go:build !windows

package gotestrunner_test

import (
	"bytes"
	"context"
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

func (s *DetachedGrandchildTestSuite) TestWaitReturnsWithinTheCeiling(t *gotest.T) {
	const runArg = "-test.run=^TestDetachedGrandchildTestSuite$/^TestWaitReturnsWithinTheCeiling$"
	switch os.Getenv("GOTEST_MP_DETACH") {
	case "child":
		// Start a session-leader grandchild on the inherited stdout and exit.
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

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, os.Args[0], runArg) //nolint:gosec // G204: test-only subprocess
	cmd.Env = append(os.Environ(), "GOTEST_MP_DETACH=child", "GOTEST_MP_STOP_FILE="+s.stopFile)
	// A buffer, as RunSingleSuite uses: exec copies through a pipe, and Wait
	// blocks until every holder of that pipe has closed it.
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	mp := gotestrunner.NewManagedProcess(cmd, gotestrunner.ProcessConfig{
		Grace:         gotestrunner.GraceFixed,
		GraceDuration: 100 * time.Millisecond,
		WaitDelay:     time.Second,
	})
	gotest.NoError(t, mp.Start())
	// The child is gone at once; the grandchild still holds its stdout.
	time.Sleep(300 * time.Millisecond)
	cancel()

	start := time.Now()
	_ = mp.WaitWithGrace(ctx)
	elapsed := time.Since(start)

	t.It("returns once the wait ceiling passes, not when the grandchild lets go", func(it *gotest.T) {
		gotest.Less(it, elapsed, 5*time.Second, "Wait blocked %v on a pipe a detached grandchild holds", elapsed)
	})
}
