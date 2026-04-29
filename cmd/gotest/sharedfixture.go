package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/mvrahden/go-test/internal/gotestgen"
)

// SharedFixtureProcess manages a running shared fixture setup subprocess.
// The subprocess starts shared fixtures, writes JSON state to stdout,
// then blocks until SIGTERM/SIGINT triggers teardown.
type SharedFixtureProcess struct {
	cmd       *exec.Cmd
	stateJSON string
	done      chan struct{}
}

// StateJSON returns the serialized shared fixture state as a JSON string.
func (p *SharedFixtureProcess) StateJSON() string { return p.stateJSON }

// Teardown signals the shared fixture subprocess to shut down and waits
// for it to complete. If the process doesn't exit within 30 seconds,
// it is forcibly killed.
func (p *SharedFixtureProcess) Teardown() error {
	if p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	p.cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-p.done:
	case <-time.After(30 * time.Second):
		p.cmd.Process.Kill()
	}
	return nil
}

// startSharedFixtures generates a shared setup binary, starts it as a subprocess,
// reads the exported env vars from stdout, and returns a SharedFixtureProcess.
func startSharedFixtures(ctx context.Context, fixtures []gotestgen.SharedFixtureInfo) (*SharedFixtureProcess, error) {
	// Generate setup binary source
	src, err := gotestgen.GenerateSharedSetup(fixtures)
	if err != nil {
		return nil, fmt.Errorf("generate shared setup: %w", err)
	}

	// Write to temp file
	tmpFile := "_gotest_shared_setup.go"
	if err := os.WriteFile(tmpFile, src, 0644); err != nil {
		return nil, err
	}
	defer os.Remove(tmpFile)

	// Start subprocess
	cmd := exec.CommandContext(ctx, "go", "run", tmpFile)
	cmd.Stderr = os.Stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start shared fixture process: %w", err)
	}

	// Read JSON state from stdout (map[string]json.RawMessage)
	var state map[string]json.RawMessage
	if err := json.NewDecoder(stdout).Decode(&state); err != nil {
		cmd.Process.Kill()
		return nil, fmt.Errorf("read shared fixture state: %w", err)
	}

	stateBytes, err := json.Marshal(state)
	if err != nil {
		cmd.Process.Kill()
		return nil, fmt.Errorf("re-marshal shared fixture state: %w", err)
	}

	done := make(chan struct{})
	go func() {
		cmd.Wait()
		close(done)
	}()

	return &SharedFixtureProcess{cmd: cmd, stateJSON: string(stateBytes), done: done}, nil
}
