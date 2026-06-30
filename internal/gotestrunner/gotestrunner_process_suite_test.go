package gotestrunner_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// GotestrunnerProcessTestSuite tests process group lifecycle,
// signal-based cancellation, and teardown budget enforcement.
type GotestrunnerProcessTestSuite struct{}

func (s *GotestrunnerProcessTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *GotestrunnerProcessTestSuite) TestGracefulTermination(t *gotest.T) {
	t.When("sending termination signal to a process", func(w *gotest.T) {
		w.It("allows the process to run cleanup before exiting", func(it *gotest.T) {
			if os.Getenv("GOTEST_GRACEFUL_CHILD") == "1" {
				marker := os.Getenv("GOTEST_MARKER_FILE")
				sig := make(chan os.Signal, 1)
				signal.Notify(sig, gracefulSignals...)
				fmt.Println("ready")
				<-sig
				_ = os.WriteFile(marker, []byte("cleanup-ran"), 0600)
				os.Exit(0)
			}

			marker := filepath.Join(it.TempDir(), "marker")

			cmd := exec.CommandContext(context.Background(), os.Args[0], //nolint:gosec // G204: test-only subprocess with hardcoded args
				"-test.run=^TestGotestrunnerProcessTestSuite$/^TestGracefulTermination$")
			cmd.Env = append(os.Environ(),
				"GOTEST_GRACEFUL_CHILD=1",
				"GOTEST_MARKER_FILE="+marker)
			gotestrunner.SetProcessGroup(cmd)
			cmd.WaitDelay = 5 * time.Second

			stdout, err := cmd.StdoutPipe()
			gotest.NoError(it, err)
			err = cmd.Start()
			gotest.NoError(it, err)

			buf := make([]byte, 256)
			n, _ := stdout.Read(buf)
			gotest.Contains(it, string(buf[:n]), "ready",
				"child not ready: %q", string(buf[:n]))

			err = gotestrunner.TerminateProcessGroup(cmd.Process.Pid)
			gotest.NoError(it, err)

			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()

			select {
			case <-done:
			case <-time.After(5 * time.Second):
				_ = cmd.Process.Kill()
				it.T().Fatal("process did not exit after TerminateProcessGroup")
			}

			data, err := os.ReadFile(marker)
			gotest.NoError(it, err)
			gotest.Equal(it, "cleanup-ran", string(data))
		})
	})
}

func (s *GotestrunnerProcessTestSuite) TestTeardownBudget(t *gotest.T) {
	t.When("reading budget file", func(w *gotest.T) {
		w.When("empty path", func(w *gotest.T) {
			w.It("returns default", func(it *gotest.T) {
				got := gotestrunner.ExportReadTeardownBudget("")
				gotest.Equal(it, gotestrunner.GracefulShutdownDelay, got)
			})
		})

		w.When("missing file", func(w *gotest.T) {
			w.It("returns default", func(it *gotest.T) {
				got := gotestrunner.ExportReadTeardownBudget("/nonexistent/budget")
				gotest.Equal(it, gotestrunner.GracefulShutdownDelay, got)
			})
		})

		w.When("valid duration", func(w *gotest.T) {
			w.It("returns parsed duration", func(it *gotest.T) {
				f := filepath.Join(it.TempDir(), "budget")
				_ = os.WriteFile(f, []byte("2m30s\n"), 0600)
				got := gotestrunner.ExportReadTeardownBudget(f)
				want := 2*time.Minute + 30*time.Second
				gotest.Equal(it, want, got)
			})
		})

		w.When("invalid duration", func(w *gotest.T) {
			w.It("returns default", func(it *gotest.T) {
				f := filepath.Join(it.TempDir(), "budget")
				_ = os.WriteFile(f, []byte("not-a-duration"), 0600)
				got := gotestrunner.ExportReadTeardownBudget(f)
				gotest.Equal(it, gotestrunner.GracefulShutdownDelay, got)
			})
		})

		w.When("zero duration", func(w *gotest.T) {
			w.It("returns default", func(it *gotest.T) {
				f := filepath.Join(it.TempDir(), "budget")
				_ = os.WriteFile(f, []byte("0s"), 0600)
				got := gotestrunner.ExportReadTeardownBudget(f)
				gotest.Equal(it, gotestrunner.GracefulShutdownDelay, got)
			})
		})
	})

	t.When("env injection in BuildSuiteCmd", func(w *gotest.T) {
		ctx := context.Background()
		env := []string{"PATH=/usr/bin"}

		w.When("BudgetFile is set", func(w *gotest.T) {
			w.It("sets GOTEST_TEARDOWN_BUDGET_FILE in env", func(it *gotest.T) {
				target := gotestrunner.SuiteTarget{
					SuiteSpec:  gotestrunner.SuiteSpec{Package: "example.com/pkg", SuiteName: "TestFoo"},
					BinaryPath: "/tmp/pkg.test",
					BudgetFile: "/tmp/pkg.test.budget",
				}
				cmd := gotestrunner.ExportBuildSuiteCmd(ctx, target, env, false)
				gotest.Contains(it, cmd.Env, protocol.EnvTeardownBudgetFile+"=/tmp/pkg.test.budget", "GOTEST_TEARDOWN_BUDGET_FILE not found in cmd.Env")
			})
		})

		w.When("BudgetFile is empty", func(w *gotest.T) {
			w.It("does not set GOTEST_TEARDOWN_BUDGET_FILE", func(it *gotest.T) {
				target := gotestrunner.SuiteTarget{
					SuiteSpec:  gotestrunner.SuiteSpec{Package: "example.com/pkg", SuiteName: "TestFoo"},
					BinaryPath: "/tmp/pkg.test",
				}
				cmd := gotestrunner.ExportBuildSuiteCmd(ctx, target, env, false)
				for _, e := range cmd.Env {
					gotest.False(it, strings.HasPrefix(e, protocol.EnvTeardownBudgetFile+"="),
						"unexpected GOTEST_TEARDOWN_BUDGET_FILE in env: %s", e)
				}
			})
		})
	})
}
