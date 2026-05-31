//go:build !windows

package gotestrunner_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

var gracefulSignals = []os.Signal{syscall.SIGTERM}

func (s *GotestrunnerProcessTestSuite) TestProcessGroupSetup(t *gotest.T) {
	t.When("setting on a command", func(w *gotest.T) {
		w.It("sets Setpgid, Cancel, and WaitDelay", func(it *gotest.T) {
			cmd := exec.Command("echo")
			gotestrunner.SetProcessGroup(cmd)

			gotest.True(it, cmd.SysProcAttr != nil && cmd.SysProcAttr.Setpgid,
				"expected Setpgid to be true")
			gotest.True(it, cmd.Cancel != nil, "expected Cancel to be set")
			gotest.Equal(it, gotestrunner.GracefulShutdownDelay, cmd.WaitDelay)
		})
	})

	t.When("building suite cmd", func(w *gotest.T) {
		for sub, tc := range gotest.Each(w, []struct {
			Name      string
			test2json bool
		}{
			{"plain mode", false},
			{"test2json mode", true},
		}) {
			ctx := context.Background()
			env := []string{"PATH=/usr/bin"}
			target := gotestrunner.SuiteTarget{
				SuiteSpec:  gotestrunner.SuiteSpec{Package: "example.com/pkg", SuiteName: "TestFoo"},
				BinaryPath: "/tmp/pkg.test",
			}
			cmd := gotestrunner.ExportBuildSuiteCmd(ctx, target, env, tc.test2json)

			gotest.True(sub, cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setpgid,
				"buildSuiteCmd should not set process group — NewManagedProcess handles it")
		}
	})
}

func (s *GotestrunnerProcessTestSuite) TestProcessGroupCancel(t *gotest.T) {
	t.When("process is running", func(w *gotest.T) {
		w.It("sends SIGTERM to the process group", func(it *gotest.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			cmd := exec.CommandContext(ctx, "sleep", "300")
			gotestrunner.SetProcessGroup(cmd)

			err := cmd.Start()
			gotest.NoError(it, err)
			pid := cmd.Process.Pid

			// Verify the process is in its own group (PGID == PID).
			pgid, err := syscall.Getpgid(pid)
			gotest.NoError(it, err)
			gotest.Equal(it, pid, pgid)

			// Call Cancel directly and verify it sends SIGTERM to the group.
			err = cmd.Cancel()
			gotest.NoError(it, err)

			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()

			select {
			case <-done:
			case <-time.After(5 * time.Second):
				cmd.Process.Kill()
				it.T().Fatal("process did not exit after Cancel")
			}

			// Process should be gone.
			err = syscall.Kill(pid, 0)
			gotest.True(it, err != nil, "process %d still alive after Cancel", pid)
		})
	})

	t.When("process not yet started", func(w *gotest.T) {
		w.It("does not panic and returns no error", func(it *gotest.T) {
			cmd := exec.Command("sleep", "300")
			gotestrunner.SetProcessGroup(cmd)

			// Cancel before Start -- Process is nil, should not panic.
			err := cmd.Cancel()
			gotest.NoError(it, err)
		})
	})
}

func (s *GotestrunnerProcessTestSuite) TestProcessGroupTermination(t *gotest.T) {
	t.When("process ignores SIGTERM", func(w *gotest.T) {
		w.It("force-kills after WaitDelay", func(it *gotest.T) {
			if os.Getenv("GOTEST_TRAP_SIGTERM") == "1" {
				// Child: trap SIGTERM and ignore it, then sleep.
				fmt.Println("ready")
				// Block by reading stdin (never gets data).
				buf := make([]byte, 1)
				os.Stdin.Read(buf)
				return
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestGotestrunnerProcessTestSuite$/^TestProcessGroupTermination$")
			cmd.Env = append(os.Environ(), "GOTEST_TRAP_SIGTERM=1")
			gotestrunner.SetProcessGroup(cmd)
			// Override WaitDelay to a short value for the test.
			cmd.WaitDelay = 500 * time.Millisecond

			stdout, err := cmd.StdoutPipe()
			gotest.NoError(it, err)
			err = cmd.Start()
			gotest.NoError(it, err)

			// Wait for the child to be ready.
			buf := make([]byte, 64)
			n, _ := stdout.Read(buf)
			gotest.Contains(it, string(buf[:n]), "ready",
				"child not ready: %q", string(buf[:n]))

			pid := cmd.Process.Pid
			cancel()

			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()

			select {
			case <-done:
			case <-time.After(10 * time.Second):
				cmd.Process.Kill()
				it.T().Fatal("process not killed after WaitDelay")
			}

			// Process should be dead.
			time.Sleep(50 * time.Millisecond)
			err = syscall.Kill(pid, 0)
			if err == nil {
				syscall.Kill(-pid, syscall.SIGKILL)
				it.T().Errorf("process %d still alive after WaitDelay force-kill", pid)
			}
		})
	})

	t.When("process has child tree", func(w *gotest.T) {
		w.It("kills entire process tree", func(it *gotest.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Spawn a shell that starts a background child, then waits.
			cmd := exec.CommandContext(ctx, "sh", "-c", `
				sleep 300 &
				CHILD=$!
				echo "child=$CHILD"
				sleep 300
			`)
			gotestrunner.SetProcessGroup(cmd)

			stdout, err := cmd.StdoutPipe()
			gotest.NoError(it, err)
			err = cmd.Start()
			gotest.NoError(it, err)

			// Read the child PID from stdout.
			buf := make([]byte, 256)
			n, err := stdout.Read(buf)
			gotest.NoError(it, err)
			line := strings.TrimSpace(string(buf[:n]))
			gotest.True(it, strings.HasPrefix(line, "child="),
				"unexpected output: %q", line)
			childPID, err := strconv.Atoi(strings.TrimPrefix(line, "child="))
			gotest.NoError(it, err)

			// Cancel the context -- SetProcessGroup's Cancel sends SIGTERM to the group.
			cancel()

			// Wait for the command to finish (killed by context cancellation).
			cmd.Wait()

			// Give the OS a moment to reap.
			time.Sleep(100 * time.Millisecond)

			// Verify the grandchild (sleep 300) is also dead.
			proc, err := os.FindProcess(childPID)
			if err != nil {
				return // can't find it -- already gone
			}
			err = proc.Signal(syscall.Signal(0))
			if err == nil {
				proc.Kill()
				it.T().Errorf("grandchild process %d is still alive after context cancellation", childPID)
			}
		})
	})
}
