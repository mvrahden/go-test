//go:build !windows

package gotestrunner //nolint:stdlib-test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestSetProcessGroup(t *testing.T) {
	cmd := exec.Command("echo")
	SetProcessGroup(cmd)

	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setpgid {
		t.Fatal("expected Setpgid to be true")
	}
	if cmd.Cancel == nil {
		t.Fatal("expected Cancel to be set")
	}
	if cmd.WaitDelay != GracefulShutdownDelay {
		t.Errorf("WaitDelay = %v, want %v", cmd.WaitDelay, GracefulShutdownDelay)
	}
}

func TestSetProcessGroup_KillsProcessTree(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Spawn a shell that starts a background child, then waits.
	// The shell and its child form a process tree we need to kill.
	cmd := exec.CommandContext(ctx, "sh", "-c", `
		sleep 300 &
		CHILD=$!
		echo "child=$CHILD"
		sleep 300
	`)
	SetProcessGroup(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	// Read the child PID from stdout.
	buf := make([]byte, 256)
	n, err := stdout.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	line := strings.TrimSpace(string(buf[:n]))
	if !strings.HasPrefix(line, "child=") {
		t.Fatalf("unexpected output: %q", line)
	}
	childPID, err := strconv.Atoi(strings.TrimPrefix(line, "child="))
	if err != nil {
		t.Fatalf("parse child PID: %v", err)
	}

	// Cancel the context — SetProcessGroup's Cancel sends SIGTERM to the group.
	cancel()

	// Wait for the command to finish (killed by context cancellation).
	cmd.Wait()

	// Give the OS a moment to reap.
	time.Sleep(100 * time.Millisecond)

	// Verify the grandchild (sleep 300) is also dead.
	proc, err := os.FindProcess(childPID)
	if err != nil {
		return // can't find it — already gone
	}
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		t.Errorf("grandchild process %d is still alive after context cancellation", childPID)
		// Clean up.
		proc.Kill()
	}
}

func TestBuildSuiteCmd_SetsProcessGroup(t *testing.T) {
	ctx := context.Background()
	env := []string{"PATH=/usr/bin"}

	for _, test2json := range []bool{false, true} {
		name := "plain"
		if test2json {
			name = "test2json"
		}
		t.Run(name, func(t *testing.T) {
			target := SuiteTarget{
				Package:    "example.com/pkg",
				BinaryPath: "/tmp/pkg.test",
				SuiteName:  "TestFoo",
			}
			cmd := buildSuiteCmd(ctx, target, env, test2json)

			if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setpgid {
				t.Fatal("expected Setpgid to be true")
			}
			if cmd.Cancel == nil {
				t.Fatal("expected Cancel to be set")
			}
			if cmd.WaitDelay != 0 {
				t.Errorf("WaitDelay = %v, want 0 (manual kill timer)", cmd.WaitDelay)
			}
		})
	}
}

func TestSetProcessGroup_Cancel_SendsSIGTERMToGroup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(ctx, "sleep", "300")
	SetProcessGroup(cmd)

	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := cmd.Process.Pid

	// Verify the process is in its own group (PGID == PID).
	pgid, err := syscall.Getpgid(pid)
	if err != nil {
		t.Fatalf("Getpgid: %v", err)
	}
	if pgid != pid {
		t.Fatalf("expected PGID %d == PID %d", pgid, pid)
	}

	// Call Cancel directly and verify it sends SIGTERM to the group.
	err = cmd.Cancel()
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("process did not exit after Cancel")
		cmd.Process.Kill()
	}

	// Process should be gone.
	err = syscall.Kill(pid, 0)
	if err == nil {
		t.Errorf("process %d still alive after Cancel", pid)
	}
}

func TestSetProcessGroup_Cancel_NilProcess(t *testing.T) {
	cmd := exec.Command("sleep", "300")
	SetProcessGroup(cmd)

	// Cancel before Start — Process is nil, should not panic.
	err := cmd.Cancel()
	if err != nil {
		t.Errorf("Cancel with nil Process returned error: %v", err)
	}
}

func TestReadTeardownBudget(t *testing.T) {
	t.Run("empty path returns default", func(t *testing.T) {
		got := readTeardownBudget("")
		if got != GracefulShutdownDelay {
			t.Errorf("got %v, want %v", got, GracefulShutdownDelay)
		}
	})

	t.Run("missing file returns default", func(t *testing.T) {
		got := readTeardownBudget("/nonexistent/budget")
		if got != GracefulShutdownDelay {
			t.Errorf("got %v, want %v", got, GracefulShutdownDelay)
		}
	})

	t.Run("valid duration", func(t *testing.T) {
		f := filepath.Join(t.TempDir(), "budget")
		os.WriteFile(f, []byte("2m30s\n"), 0644)
		got := readTeardownBudget(f)
		want := 2*time.Minute + 30*time.Second
		if got != want {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("invalid duration returns default", func(t *testing.T) {
		f := filepath.Join(t.TempDir(), "budget")
		os.WriteFile(f, []byte("not-a-duration"), 0644)
		got := readTeardownBudget(f)
		if got != GracefulShutdownDelay {
			t.Errorf("got %v, want %v", got, GracefulShutdownDelay)
		}
	})

	t.Run("zero duration returns default", func(t *testing.T) {
		f := filepath.Join(t.TempDir(), "budget")
		os.WriteFile(f, []byte("0s"), 0644)
		got := readTeardownBudget(f)
		if got != GracefulShutdownDelay {
			t.Errorf("got %v, want %v", got, GracefulShutdownDelay)
		}
	})
}

func TestBuildSuiteCmd_BudgetFileEnv(t *testing.T) {
	ctx := context.Background()
	env := []string{"PATH=/usr/bin"}

	t.Run("sets GOTEST_TEARDOWN_BUDGET_FILE when BudgetFile is set", func(t *testing.T) {
		target := SuiteTarget{
			Package:    "example.com/pkg",
			BinaryPath: "/tmp/pkg.test",
			SuiteName:  "TestFoo",
			BudgetFile: "/tmp/pkg.test.budget",
		}
		cmd := buildSuiteCmd(ctx, target, env, false)
		found := false
		for _, e := range cmd.Env {
			if e == "GOTEST_TEARDOWN_BUDGET_FILE=/tmp/pkg.test.budget" {
				found = true
				break
			}
		}
		if !found {
			t.Error("GOTEST_TEARDOWN_BUDGET_FILE not found in cmd.Env")
		}
	})

	t.Run("no GOTEST_TEARDOWN_BUDGET_FILE when BudgetFile is empty", func(t *testing.T) {
		target := SuiteTarget{
			Package:    "example.com/pkg",
			BinaryPath: "/tmp/pkg.test",
			SuiteName:  "TestFoo",
		}
		cmd := buildSuiteCmd(ctx, target, env, false)
		for _, e := range cmd.Env {
			if strings.HasPrefix(e, "GOTEST_TEARDOWN_BUDGET_FILE=") {
				t.Errorf("unexpected GOTEST_TEARDOWN_BUDGET_FILE in env: %s", e)
			}
		}
	})
}

func TestSetProcessGroup_WaitDelay_ForceKills(t *testing.T) {
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

	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSetProcessGroup_WaitDelay_ForceKills$")
	cmd.Env = append(os.Environ(), "GOTEST_TRAP_SIGTERM=1")
	SetProcessGroup(cmd)
	// Override WaitDelay to a short value for the test.
	cmd.WaitDelay = 500 * time.Millisecond

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	// Wait for the child to be ready.
	buf := make([]byte, 64)
	n, _ := stdout.Read(buf)
	if !strings.Contains(string(buf[:n]), "ready") {
		t.Fatalf("child not ready: %q", string(buf[:n]))
	}

	pid := cmd.Process.Pid
	cancel()

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("process not killed after WaitDelay")
		cmd.Process.Kill()
	}

	// Process should be dead.
	time.Sleep(50 * time.Millisecond)
	err = syscall.Kill(pid, 0)
	if err == nil {
		t.Errorf("process %d still alive after WaitDelay force-kill", pid)
		syscall.Kill(-pid, syscall.SIGKILL)
	}
}
