package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/mvrahden/go-test/internal/gotestgen"
)

// SharedFixtureProcess manages a running shared fixture setup subprocess.
// The subprocess starts shared fixtures, writes JSON state to stdout,
// then blocks until SIGTERM/SIGINT triggers teardown.
type SharedFixtureProcess struct {
	cmd       *exec.Cmd
	stateFile string
	done      chan struct{}
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
	p.cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-p.done:
	case <-time.After(30 * time.Second):
		p.cmd.Process.Kill()
	}
	return nil
}

// startSharedFixtures generates a shared setup binary in the overlay temp dir,
// starts it as a subprocess, reads JSON state from stdout, writes it to a state
// file, and returns a SharedFixtureProcess.
func startSharedFixtures(ctx context.Context, tmpDir string, fixtures []gotestgen.SharedFixtureInfo) (*SharedFixtureProcess, error) {
	src, err := gotestgen.GenerateSharedSetup(fixtures)
	if err != nil {
		return nil, fmt.Errorf("generate shared setup: %w", err)
	}

	sharedDir := filepath.Join(tmpDir, "shared")
	if err := os.MkdirAll(sharedDir, 0755); err != nil {
		return nil, fmt.Errorf("create shared fixture dir: %w", err)
	}

	// Write setup source as a real package inside the module tree so that
	// `go build` treats it as a module-internal package, allowing imports
	// of internal/ packages.  We remove the directory after building.
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getwd: %w", err)
	}
	setupPkgDir := filepath.Join(cwd, "gotest_shared_setup_")
	if err := os.MkdirAll(setupPkgDir, 0755); err != nil {
		return nil, fmt.Errorf("create setup pkg dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(setupPkgDir, "main.go"), src, 0644); err != nil {
		os.RemoveAll(setupPkgDir)
		return nil, err
	}

	setupBin := filepath.Join(sharedDir, "setup")
	buildCmd := exec.CommandContext(ctx, "go", "build", "-o", setupBin, "./gotest_shared_setup_/")
	buildCmd.Dir = cwd
	buildCmd.Stderr = os.Stderr
	if err := buildCmd.Run(); err != nil {
		os.RemoveAll(setupPkgDir)
		return nil, fmt.Errorf("build shared fixture setup: %w", err)
	}
	os.RemoveAll(setupPkgDir)

	cmd := exec.CommandContext(ctx, setupBin)
	cmd.Stderr = os.Stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start shared fixture process: %w", err)
	}

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

	return &SharedFixtureProcess{cmd: cmd, stateFile: stateFile, done: done}, nil
}
