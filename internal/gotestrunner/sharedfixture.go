package gotestrunner

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/mvrahden/go-test/internal/gotestgen"
)

// sharedTeardownFailedExit is the status the generated setup program exits with
// when one or more shared fixtures failed to tear down. It must stay in step
// with the value used by internal/gotestgen/static/sharedfixture.go.tpl.
const sharedTeardownFailedExit = 1

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
	sharedDir       string
	done            chan struct{}
	teardownTimeout time.Duration

	ready    map[string]chan struct{}
	state    map[string]json.RawMessage
	mu       sync.Mutex
	allDone  chan struct{}
	setupErr error
	waitErr  error // exit status of the subprocess; valid once done is closed

	stdin   io.WriteCloser // command channel for window verbs (StartKeys/TeardownKeys)
	cmdMu   sync.Mutex     // serializes commands: one in flight, one _cmd ack each
	cmdResp chan string    // _cmd ack payloads (error text, "" on success)
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

// WaitAllReady blocks until all fixtures have completed setup (the _done sentinel
// is received). On success it writes the accumulated state to a global state file.
// Returns error on setup failure, timeout, or context cancellation.
//
// Every failure path shuts the subprocess down gracefully instead of killing it.
// The subprocess owns its teardown: on a setup failure it is holding the sibling
// fixtures that did come up, and a SIGKILL here — the old behavior — cut their
// AfterAlls short and leaked whatever they held.
func (p *SharedFixtureProcess) WaitAllReady(ctx context.Context, timeout time.Duration) error {
	var deadline <-chan time.Time
	if timeout > 0 {
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		deadline = timer.C
	}
	select {
	case <-p.allDone:
	case <-ctx.Done():
		p.shutdown()
		return fmt.Errorf("cancelled: %w", ctx.Err())
	case <-deadline:
		p.shutdown()
		return fmt.Errorf("timed out after %v", timeout)
	}
	if p.setupErr != nil {
		p.shutdown()
		return fmt.Errorf("shared fixture setup: %w", p.setupErr)
	}

	p.mu.Lock()
	stateBytes, err := json.Marshal(p.state)
	p.mu.Unlock()
	if err != nil {
		p.shutdown()
		return fmt.Errorf("re-marshal shared fixture state: %w", err)
	}

	p.stateFile = filepath.Join(p.sharedDir, "state.json")
	if err := os.WriteFile(p.stateFile, stateBytes, 0600); err != nil {
		p.shutdown()
		return fmt.Errorf("write shared fixture state file: %w", err)
	}
	return nil
}

// shutdown asks the subprocess to exit and waits it out. Requesting and then
// waiting — rather than killing — is the runner's half of the shutdown
// contract; force-kill at the budget deadline is its only other lever.
func (p *SharedFixtureProcess) shutdown() {
	if p.cmd == nil || p.cmd.Process == nil {
		return
	}
	_ = TerminateProcessGroup(p.cmd.Process.Pid)
	p.awaitExit(p.teardownBudget())
}

// awaitExit waits up to budget for the subprocess to exit on its own,
// force-killing the group when it does not, and reports whether force was
// needed.
func (p *SharedFixtureProcess) awaitExit(budget time.Duration) (forceKilled bool) {
	select {
	case <-p.done:
		return false
	case <-time.After(budget):
		fmt.Fprintf(os.Stderr, "WARN: shared fixture process did not exit within %v, forcing termination\n", budget)
		_ = ForceKillProcessGroup(p.cmd.Process.Pid)
		<-p.done
		return true
	}
}

// teardownBudget returns the budget the subprocess reported on its _done line,
// floored to 30 seconds when none arrived.
func (p *SharedFixtureProcess) teardownBudget() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.teardownTimeout <= 0 {
		return 30 * time.Second
	}
	return p.teardownTimeout
}

// WriteStateFileForKeys writes a state file containing only the specified keys.
// Returns the path to the written file.
func (p *SharedFixtureProcess) WriteStateFileForKeys(name string, keys []string) (string, error) {
	p.mu.Lock()
	subset := make(map[string]json.RawMessage, len(keys))
	for _, k := range keys {
		if v, ok := p.state[k]; ok {
			subset[k] = v
		}
	}
	p.mu.Unlock()

	data, err := json.Marshal(subset)
	if err != nil {
		return "", fmt.Errorf("marshal state for %s: %w", name, err)
	}

	path := filepath.Join(p.sharedDir, name+".json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		return "", fmt.Errorf("write state file for %s: %w", name, err)
	}
	return path, nil
}

// StartKeys asks the setup subprocess to start the given fixtures now — the
// opening half of a window boundary. The subprocess runs them in dependency
// order under the same per-fixture retry and budget policy as the up-front
// phase, and streams their state lines before acknowledging. Blocks until the
// acknowledgement; timeout 0 means no deadline.
func (p *SharedFixtureProcess) StartKeys(keys []string, timeout time.Duration) error {
	return p.command("start", keys, timeout)
}

// TeardownKeys asks the setup subprocess to tear down the given fixtures now,
// in reverse dependency order — the releasing half of a window boundary.
// Teardown (the terminal owner) later skips keys already released here.
// Blocks until the acknowledgement; timeout 0 means no deadline.
func (p *SharedFixtureProcess) TeardownKeys(keys []string, timeout time.Duration) error {
	return p.command("teardown", keys, timeout)
}

func (p *SharedFixtureProcess) command(verb string, keys []string, timeout time.Duration) error {
	if len(keys) == 0 {
		return nil
	}
	if p.stdin == nil {
		return fmt.Errorf("shared fixture process has no command channel")
	}
	p.cmdMu.Lock()
	defer p.cmdMu.Unlock()

	// Drain a stale ack a timed-out predecessor left behind: every command
	// gets exactly one ack, and it must be its own.
	select {
	case <-p.cmdResp:
	default:
	}

	line, err := json.Marshal(struct {
		Verb string   `json:"verb"`
		Keys []string `json:"keys"`
	}{Verb: verb, Keys: keys})
	if err != nil {
		return err
	}
	if _, err := p.stdin.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("send %s to shared fixture process: %w", verb, err)
	}

	var deadline <-chan time.Time
	if timeout > 0 {
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		deadline = timer.C
	}
	select {
	case msg := <-p.cmdResp:
		if msg != "" {
			return fmt.Errorf("shared fixture %s: %s", verb, msg)
		}
		return nil
	case <-p.done:
		return fmt.Errorf("shared fixture process exited before acknowledging %s", verb)
	case <-deadline:
		return fmt.Errorf("shared fixture %s timed out after %v", verb, timeout)
	}
}

// RefreshStateFile rewrites the global state file (the one WaitAllReady wrote
// and every batch suite's env points at) from the current accumulated state —
// after StartKeys added late fixtures, the file must contain them.
func (p *SharedFixtureProcess) RefreshStateFile() error {
	if p.stateFile == "" {
		return nil
	}
	p.mu.Lock()
	data, err := json.Marshal(p.state)
	p.mu.Unlock()
	if err != nil {
		return fmt.Errorf("re-marshal shared fixture state: %w", err)
	}
	if err := os.WriteFile(p.stateFile, data, 0600); err != nil {
		return fmt.Errorf("refresh shared fixture state file: %w", err)
	}
	return nil
}

// Teardown signals the shared fixture subprocess to shut down and waits for it
// to complete within its teardown budget (30 seconds if none was reported). A
// process that outlives the budget is forcibly killed, and that is reported as
// an error: its AfterAll never finished, so what it held is leaked.
//
// Success is proven, never inferred: past the setup and force-kill gates, only
// a clean exit shows the teardown epilogue ran and passed. Recognizing one
// specific failure status and defaulting to green — the old shape — let a
// panic in a fixture-owned goroutine (exit 2) or an external kill (no status)
// during the teardown window read as success while containers stayed up.
func (p *SharedFixtureProcess) Teardown() error {
	if p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	budget := p.teardownBudget()

	// Noted before signalling only to sharpen the message below: an exit that
	// predates the shutdown request happened while tests may still have been
	// running, which is a different story than dying mid-teardown.
	diedEarly := false
	select {
	case <-p.done:
		diedEarly = true
	default:
	}

	_ = TerminateProcessGroup(p.cmd.Process.Pid)
	forceKilled := p.awaitExit(budget)
	if p.sharedDir != "" {
		os.RemoveAll(p.sharedDir)
	}

	// A setup failure already surfaced through SetupErr; the subprocess ran its
	// sibling teardowns under the runner's shutdown request and exited 1 for
	// the setup failure. Reporting that status here again would blame teardown
	// for it.
	if p.setupErr != nil {
		return nil
	}

	// A force-killed process was cut off mid-AfterAll. Its exit status says
	// nothing — a signalled process reports -1, never sharedTeardownFailedExit —
	// so without reporting it here the run would end green while the containers
	// the fixture was still stopping stay up.
	if forceKilled {
		return fmt.Errorf("shared fixture teardown was force-killed after %s; its AfterAll never finished and its resources may be leaked", budget)
	}

	// A clean exit is the only proof of a completed teardown: the epilogue runs
	// on any shutdown signal — whoever sent it — and exits 0 only when every
	// AfterAll passed. Everything else fails, with the most precise message the
	// status supports. exec maps a success status under a canceled command
	// context to ctx.Err() — and only a success status, so a context error here
	// IS the clean exit, just relabeled.
	if p.waitErr == nil || errors.Is(p.waitErr, context.Canceled) || errors.Is(p.waitErr, context.DeadlineExceeded) {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(p.waitErr, &exitErr) && exitErr.ExitCode() == sharedTeardownFailedExit {
		return fmt.Errorf("shared fixture teardown failed; see AfterAll errors above")
	}
	if diedEarly {
		return fmt.Errorf("shared fixture process exited before teardown was requested (%v); its AfterAll may not have run and its resources may be leaked", p.waitErr)
	}
	return fmt.Errorf("shared fixture process died during teardown (%v); its AfterAll may not have finished and its resources may be leaked", p.waitErr)
}

// StartSharedFixtures generates a shared setup binary in the overlay temp dir,
// starts it as a subprocess, and returns a SharedFixtureProcess immediately.
// A background goroutine reads JSON state lines from stdout, closing per-fixture
// ready channels as each fixture completes. The caller must either wait on
// individual Ready() channels (streaming) or call WaitAllReady (batch).
// setupTimeout is stored as the initial teardownTimeout; 0 means no deadline.
func StartSharedFixtures(ctx context.Context, tmpDir string, fixtures []gotestgen.SharedFixtureInfo, setupTimeout time.Duration) (*SharedFixtureProcess, error) {
	src, err := gotestgen.GenerateSharedSetup(fixtures)
	if err != nil {
		return nil, fmt.Errorf("generate shared setup: %w", err)
	}

	sharedDir := filepath.Join(tmpDir, "shared")
	if err := os.MkdirAll(sharedDir, 0755); err != nil {
		return nil, fmt.Errorf("create shared fixture dir: %w", err)
	}

	setupFile := filepath.Join(sharedDir, "setup.go")
	if err := os.WriteFile(setupFile, src, 0600); err != nil {
		return nil, err
	}

	setupBin := filepath.Join(sharedDir, "setup")
	if runtime.GOOS == "windows" {
		setupBin += ".exe"
	}
	defer logSlowBuild(os.Stderr, "shared fixture setup binary", slowBuildThreshold)()
	buildCmd := exec.CommandContext(ctx, "go", "build", "-o", setupBin, setupFile)
	buildCmd.Stderr = os.Stderr
	buildMp := NewManagedProcess(buildCmd, ProcessConfig{Grace: GraceKill})
	if err := buildMp.Start(); err != nil {
		return nil, fmt.Errorf("build shared fixture setup: %w", err)
	}
	if err := buildMp.WaitWithGrace(ctx); err != nil {
		return nil, fmt.Errorf("build shared fixture setup: %w", err)
	}

	cmd := exec.CommandContext(ctx, setupBin)
	cmd.Stderr = os.Stderr

	// SetProcessGroup's WaitDelay is the backstop for a process that ignores
	// SIGTERM, and it has to stay looser than any teardown budget. Tighten it
	// below one and it becomes the budget: a fixture given minutes to stop its
	// containers is killed part-way through instead, and because a signalled
	// process reports no meaningful exit status, the run still says ok.
	SetProcessGroup(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start shared fixture process: %w", err)
	}

	// Build per-fixture readiness channels.
	ready := make(map[string]chan struct{}, len(fixtures))
	for i := range fixtures {
		key := fixtures[i].PkgPath + "." + fixtures[i].Identifier
		ready[key] = make(chan struct{})
	}

	allDone := make(chan struct{})
	state := make(map[string]json.RawMessage)
	waitDone := make(chan struct{})

	proc := &SharedFixtureProcess{
		cmd:             cmd,
		sharedDir:       sharedDir,
		done:            waitDone,
		teardownTimeout: setupTimeout,
		ready:           ready,
		state:           state,
		allDone:         allDone,
		stdin:           stdin,
		cmdResp:         make(chan string, 1),
	}

	go func() {
		defer func() {
			proc.waitErr = cmd.Wait()
			close(waitDone)
		}()
		closedReady := make(map[string]bool, len(ready))
		doneSeen := false
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		// The scan outlives _done: window verbs keep the stream alive with
		// late state lines (deferred StartKeys) and _cmd acknowledgements,
		// until EOF at process exit.
		for scanner.Scan() {
			var entry fixtureStateEntry
			if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
				continue
			}
			if entry.Key == "_done" {
				if entry.Error != "" {
					proc.setupErr = fmt.Errorf("%s", entry.Error)
				}
				if entry.TeardownBudget != "" {
					if d, err := time.ParseDuration(entry.TeardownBudget); err == nil && d > 0 {
						// Under proc.mu: an interrupted run can reach Teardown
						// while this goroutine is still draining a late _done.
						proc.mu.Lock()
						proc.teardownTimeout = d
						proc.mu.Unlock()
					}
				}
				doneSeen = true
				close(allDone)
				continue
			}
			if entry.Key == "_cmd" {
				// Ack for the one in-flight command; drop it if no one waits
				// (the commander timed out) rather than stall the scan.
				select {
				case proc.cmdResp <- entry.Error:
				default:
				}
				continue
			}
			proc.mu.Lock()
			state[entry.Key] = entry.State
			proc.mu.Unlock()
			if ch, ok := ready[entry.Key]; ok && !closedReady[entry.Key] {
				close(ch)
				closedReady[entry.Key] = true
			}
		}
		if doneSeen {
			return
		}
		if err := scanner.Err(); err != nil {
			proc.setupErr = fmt.Errorf("reading subprocess stdout: %w", err)
		} else if proc.setupErr == nil {
			proc.setupErr = fmt.Errorf("subprocess exited without _done sentinel")
		}
		close(allDone)
	}()

	return proc, nil
}
