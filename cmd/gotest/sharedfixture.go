package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
)

// SharedFixtureProcess manages a running shared fixture setup subprocess.
// The subprocess starts shared fixtures, writes JSON state to stdout,
// then blocks until SIGTERM/SIGINT triggers teardown.
type SharedFixtureProcess struct {
	cmd              *exec.Cmd
	stateFile        string
	done             chan struct{}
	teardownTimeout  time.Duration
}

// StateFile returns the path to the shared fixture state JSON file.
func (p *SharedFixtureProcess) StateFile() string { return p.stateFile }

// Teardown signals the shared fixture subprocess to shut down and waits
// for it to complete. If the process doesn't exit within 30 seconds,
// it is forcibly killed.
func (p *SharedFixtureProcess) Teardown() error {
	if p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	timeout := p.teardownTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	gotestrunner.TerminateProcessGroup(p.cmd.Process.Pid)
	select {
	case <-p.done:
	case <-time.After(timeout):
		gotestrunner.ForceKillProcessGroup(p.cmd.Process.Pid)
	}
	return nil
}

// startSharedFixtures generates a shared setup binary in the overlay temp dir,
// starts it as a subprocess, reads JSON state from stdout, writes it to a state
// file, and returns a SharedFixtureProcess.
func startSharedFixtures(ctx context.Context, tmpDir string, fixtures []gotestgen.SharedFixtureInfo, setupTimeout time.Duration) (*SharedFixtureProcess, error) {
	src, err := gotestgen.GenerateSharedSetup(fixtures)
	if err != nil {
		return nil, fmt.Errorf("generate shared setup: %w", err)
	}

	sharedDir := filepath.Join(tmpDir, "shared")
	if err := os.MkdirAll(sharedDir, 0755); err != nil {
		return nil, fmt.Errorf("create shared fixture dir: %w", err)
	}

	setupFile := filepath.Join(sharedDir, "setup.go")
	if err := os.WriteFile(setupFile, src, 0644); err != nil {
		return nil, err
	}

	setupBin := filepath.Join(sharedDir, "setup")
	buildCmd := exec.CommandContext(ctx, "go", "build", "-o", setupBin, setupFile)
	buildCmd.Stderr = os.Stderr
	gotestrunner.SetProcessGroup(buildCmd)
	buildCmd.WaitDelay = gotestrunner.BuildShutdownDelay
	if err := buildCmd.Run(); err != nil {
		return nil, fmt.Errorf("build shared fixture setup: %w", err)
	}

	cmd := exec.CommandContext(ctx, setupBin)
	cmd.Stderr = os.Stderr
	gotestrunner.SetProcessGroup(cmd)
	cmd.WaitDelay = 0 // Teardown() manages lifecycle via explicit SIGTERM/SIGKILL

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start shared fixture process: %w", err)
	}

	type decodeResult struct {
		state map[string]json.RawMessage
		err   error
	}
	decoded := make(chan decodeResult, 1)
	go func() {
		var state map[string]json.RawMessage
		err := json.NewDecoder(stdout).Decode(&state)
		decoded <- decodeResult{state, err}
	}()

	var state map[string]json.RawMessage
	select {
	case res := <-decoded:
		if res.err != nil {
			cmd.Process.Kill()
			return nil, fmt.Errorf("read shared fixture state: %w", res.err)
		}
		state = res.state
	case <-ctx.Done():
		cmd.Process.Kill()
		return nil, fmt.Errorf("cancelled: %w", ctx.Err())
	case <-time.After(setupTimeout):
		cmd.Process.Kill()
		io.Copy(io.Discard, stdout)
		return nil, fmt.Errorf("timed out after %v", setupTimeout)
	}

	teardownTimeout := setupTimeout
	if budgetRaw, ok := state["_teardownBudget"]; ok {
		var budgetStr string
		if err := json.Unmarshal(budgetRaw, &budgetStr); err == nil {
			if d, err := time.ParseDuration(budgetStr); err == nil && d > 0 {
				teardownTimeout = d
			}
		}
		delete(state, "_teardownBudget")
	}

	stateBytes, err := json.Marshal(state)
	if err != nil {
		cmd.Process.Kill()
		return nil, fmt.Errorf("re-marshal shared fixture state: %w", err)
	}

	stateFile := filepath.Join(sharedDir, "state.json")
	if err := os.WriteFile(stateFile, stateBytes, 0644); err != nil {
		cmd.Process.Kill()
		return nil, fmt.Errorf("write shared fixture state file: %w", err)
	}

	done := make(chan struct{})
	go func() {
		cmd.Wait()
		close(done)
	}()

	return &SharedFixtureProcess{cmd: cmd, stateFile: stateFile, done: done, teardownTimeout: teardownTimeout}, nil
}
