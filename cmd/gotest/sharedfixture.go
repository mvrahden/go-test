package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
)

// fixtureStateEntry represents a single JSON line emitted by the shared fixture subprocess.
type fixtureStateEntry struct {
	Key            string          `json:"key"`
	State          json.RawMessage `json:"state,omitempty"`
	TeardownBudget string          `json:"teardownBudget,omitempty"`
	Error          string          `json:"error,omitempty"`
}

// SharedFixtureProcess manages a running shared fixture setup subprocess.
// The subprocess starts shared fixtures, writes JSON state to stdout,
// then blocks until SIGTERM/SIGINT triggers teardown.
type SharedFixtureProcess struct {
	cmd             *exec.Cmd
	stateFile       string
	done            chan struct{}
	teardownTimeout time.Duration

	// Per-fixture readiness tracking.
	ready    map[string]chan struct{}    // state key → closed when fixture ready
	state    map[string]json.RawMessage // accumulated state
	mu       sync.Mutex
	allDone  chan struct{} // closed when _done received
	setupErr error        // non-nil if _done had error
}

// StateFile returns the path to the shared fixture state JSON file.
func (p *SharedFixtureProcess) StateFile() string { return p.stateFile }

// Ready returns the readiness channel for a given fixture state key.
// The channel is closed when that fixture's state has been received.
func (p *SharedFixtureProcess) Ready(key string) <-chan struct{} {
	return p.ready[key]
}

// AllDone returns a channel that closes when the subprocess has finished all setup.
func (p *SharedFixtureProcess) AllDone() <-chan struct{} {
	return p.allDone
}

// SetupErr returns the setup error, if any. Only valid after AllDone() closes.
func (p *SharedFixtureProcess) SetupErr() error {
	return p.setupErr
}

// State returns a snapshot of the accumulated state for the given keys.
func (p *SharedFixtureProcess) State(keys []string) map[string]json.RawMessage {
	p.mu.Lock()
	defer p.mu.Unlock()
	result := make(map[string]json.RawMessage, len(keys))
	for _, k := range keys {
		if v, ok := p.state[k]; ok {
			result[k] = v
		}
	}
	return result
}

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
// file, and returns a SharedFixtureProcess. setupTimeout is a total budget for
// all shared fixtures to complete setup; 0 means no external deadline (each
// fixture's own SharedFixtureConfig().Timeout governs); any negative value
// explicitly disables the deadline.
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
	if runtime.GOOS == "windows" {
		setupBin += ".exe"
	}
	buildCmd := exec.CommandContext(ctx, "go", "build", "-o", setupBin, setupFile)
	buildCmd.Stderr = os.Stderr
	gotestrunner.SetBuildProcessGroup(buildCmd)
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

	// Build per-fixture readiness channels.
	ready := make(map[string]chan struct{}, len(fixtures))
	for _, sf := range fixtures {
		key := sf.PkgPath + "." + sf.Identifier
		ready[key] = make(chan struct{})
	}

	allDone := make(chan struct{})
	state := make(map[string]json.RawMessage)
	var mu sync.Mutex
	var setupErr error
	teardownTimeout := setupTimeout

	// Read streaming JSON lines from the subprocess stdout.
	go func() {
		closedReady := make(map[string]bool, len(ready))
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			var entry fixtureStateEntry
			if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
				continue
			}
			if entry.Key == "_done" {
				if entry.Error != "" {
					setupErr = fmt.Errorf("%s", entry.Error)
				}
				if entry.TeardownBudget != "" {
					if d, err := time.ParseDuration(entry.TeardownBudget); err == nil && d > 0 {
						teardownTimeout = d
					}
				}
				close(allDone)
				return
			}
			mu.Lock()
			state[entry.Key] = entry.State
			mu.Unlock()
			if ch, ok := ready[entry.Key]; ok && !closedReady[entry.Key] {
				close(ch)
				closedReady[entry.Key] = true
			}
		}
		// Scanner ended without _done — subprocess crashed.
		if err := scanner.Err(); err != nil {
			setupErr = fmt.Errorf("reading subprocess stdout: %w", err)
		} else if setupErr == nil {
			setupErr = fmt.Errorf("subprocess exited without _done sentinel")
		}
		close(allDone)
	}()

	// Wait for allDone (with timeout).
	if setupTimeout > 0 {
		select {
		case <-allDone:
		case <-ctx.Done():
			cmd.Process.Kill()
			return nil, fmt.Errorf("cancelled: %w", ctx.Err())
		case <-time.After(setupTimeout):
			cmd.Process.Kill()
			io.Copy(io.Discard, stdout)
			return nil, fmt.Errorf("timed out after %v", setupTimeout)
		}
	} else {
		select {
		case <-allDone:
		case <-ctx.Done():
			cmd.Process.Kill()
			return nil, fmt.Errorf("cancelled: %w", ctx.Err())
		}
	}

	if setupErr != nil {
		cmd.Process.Kill()
		return nil, fmt.Errorf("shared fixture setup: %w", setupErr)
	}

	// Write accumulated state to file (backward compat with non-streaming callers).
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

	waitDone := make(chan struct{})
	go func() {
		cmd.Wait()
		close(waitDone)
	}()

	return &SharedFixtureProcess{
		cmd:             cmd,
		stateFile:       stateFile,
		done:            waitDone,
		teardownTimeout: teardownTimeout,
		ready:           ready,
		state:           state,
		allDone:         allDone,
	}, nil
}
