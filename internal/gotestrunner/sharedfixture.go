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
	"github.com/mvrahden/go-test/internal/proctree"
	"github.com/mvrahden/go-test/internal/protocol"
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
	Budget         string          `json:"budget,omitempty"` // _setup: what the fixture's setup may take
	Error          string          `json:"error,omitempty"`
	ID             uint64          `json:"id,omitempty"` // _cmd: the command it acknowledges
}

// SetupTimeoutError is the wait for shared fixtures giving up. After is the
// budget the wait was held to, not the wall clock it read.
type SetupTimeoutError struct{ After time.Duration }

func (e *SetupTimeoutError) Error() string {
	return fmt.Sprintf("shared fixture setup timed out after %v", e.After)
}

// setupReportFloor is how long the wait for shared fixtures lasts before the
// process has reported a single fixture's budget: the process starting and
// deriving its first SharedFixtureConfig. Variable for the runner's own tests.
var setupReportFloor = 30 * time.Second

// unreportedTeardownBudget is how long a process whose setup follows the
// fixtures' budgets gets to exit before it reports a teardown budget.
const unreportedTeardownBudget = 2 * time.Minute

// SharedFixtureProcess manages a running shared fixture setup subprocess.
// The subprocess starts shared fixtures, writes JSON state to stdout,
// then blocks until SIGTERM/SIGINT triggers teardown.
type SharedFixtureProcess struct {
	cmd       *exec.Cmd
	tree      *proctree.Tree
	stateFile string
	sharedDir string
	done      chan struct{}
	// teardownTimeout is the budget the process last reported. Until then it
	// is the flat setup timeout: 0 when the setup follows the fixtures'
	// budgets, negative when it has no deadline.
	teardownTimeout time.Duration

	ready    map[string]chan struct{}
	state    map[string]json.RawMessage
	mu       sync.Mutex
	allDone  chan struct{}
	setupErr error
	protoErr error // under mu: a stream the process got wrong after its setup was read
	waitErr  error // exit status of the subprocess; valid once done is closed

	// Setup budgets the process reported, for the wait that has no flat
	// timeout: the deadline moves to the latest one, and an unbounded
	// fixture lifts it. budgetSeen wakes the waiter. A command starting
	// fixtures resets them: its wait follows only the setups it started.
	// setupBegan is when those setups began: the process starting, or the
	// command being sent. The floor is timed from it, since a report can
	// arrive before the waiter starts.
	setupBegan     time.Time
	setupDeadline  time.Time     // the latest reported budget's end; zero until one was reported
	setupBudget    time.Duration // the budget that ends at setupDeadline
	setupUnbounded bool
	budgetSeen     chan struct{}
	// teardownSeen wakes a waiting teardown when a command's acknowledgement
	// reports a new teardown budget: a fixture started late came up.
	teardownSeen chan struct{}

	stdin io.WriteCloser // command channel for window verbs (StartKeys/TeardownKeys)
	// Commands run concurrently in the process; each waits for the ack
	// carrying its id. A command given up drops its waiter, and its late ack
	// with it. Under cmdMu.
	cmdMu      sync.Mutex
	cmdNext    uint64
	cmdWaiters map[uint64]chan string

	// failures keeps what the process reported on stderr about fixtures
	// that failed.
	failures *failureLog
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

// TakeFailures returns the failures fixtures reported since the last call.
// Reports are complete once the process has exited.
func (p *SharedFixtureProcess) TakeFailures() []string {
	if p.failures == nil {
		return nil
	}
	return p.failures.take()
}

// Came reports whether the fixture with the given state key is up.
func (p *SharedFixtureProcess) Came(key string) bool {
	ch := p.ready[key]
	if ch == nil {
		return false
	}
	select {
	case <-ch:
		return true
	default:
		return false
	}
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

// WaitSetup waits for the process to report its setup done, for at most
// flat when positive. Zero flat makes the wait the fixtures' own: every
// fixture reports the budget its setup is held to as it starts, and the
// deadline moves to the latest of them, with [setupReportFloor] before the
// first report; a fixture without a Timeout lifts the deadline. Negative
// flat waits without one. A setup that failed is returned as its error, a
// wait given up as a [*SetupTimeoutError].
func (p *SharedFixtureProcess) WaitSetup(ctx context.Context, flat time.Duration) error {
	clock := p.startSetupClock(flat)
	defer clock.stop()
	for {
		select {
		case <-p.allDone:
			if err := p.SetupErr(); err != nil {
				return fmt.Errorf("shared fixture setup failed: %w", err)
			}
			return nil
		case <-ctx.Done():
			return fmt.Errorf("cancelled: %w", ctx.Err())
		case <-clock.C:
			err := clock.expired()
			if flat <= 0 {
				// The process has not reported its teardown budget yet; a
				// setup that overran what it declared gets as long again to
				// stop, until _done reports the budget (see awaitExit).
				p.mu.Lock()
				if p.teardownTimeout <= 0 {
					p.teardownTimeout = max(err.After, 5*time.Second)
				}
				p.mu.Unlock()
			}
			return err
		case <-p.budgetSeen:
			clock.follow()
		}
	}
}

// setupClock is the deadline of a wait for setups the process runs: flat when
// positive, none when negative, and at zero the budgets the process reports
// as each setup starts, with [setupReportFloor] before the first.
type setupClock struct {
	p     *SharedFixtureProcess
	flat  time.Duration
	began time.Time
	after time.Duration // the budget the armed deadline ends
	timer *time.Timer
	C     <-chan time.Time // nil while the wait has no deadline
}

func (p *SharedFixtureProcess) startSetupClock(flat time.Duration) *setupClock {
	c := &setupClock{p: p, flat: flat, began: time.Now(), timer: time.NewTimer(time.Hour)}
	c.timer.Stop()
	if flat == 0 {
		p.mu.Lock()
		if !p.setupBegan.IsZero() {
			c.began = p.setupBegan
		}
		p.mu.Unlock()
	}
	switch {
	case flat > 0:
		c.arm(c.began.Add(flat), flat)
	case flat == 0:
		c.follow()
	}
	return c
}

func (c *setupClock) arm(at time.Time, budget time.Duration) {
	c.after = budget
	c.timer.Stop()
	c.timer.Reset(time.Until(at))
	c.C = c.timer.C
}

// follow re-arms at the latest reported budget, or lifts the deadline for an
// unbounded fixture. A flat wait ignores the reports.
func (c *setupClock) follow() {
	if c.flat != 0 {
		return
	}
	c.p.mu.Lock()
	unbounded, at, budget := c.p.setupUnbounded, c.p.setupDeadline, c.p.setupBudget
	c.p.mu.Unlock()
	switch {
	case unbounded:
		c.timer.Stop()
		c.C = nil
	case at.IsZero():
		c.arm(c.began.Add(setupReportFloor), setupReportFloor)
	default:
		c.arm(at, budget)
	}
}

func (c *setupClock) stop() { c.timer.Stop() }

// expired names the budget the wait was held to: a report read late, or a
// timer fired late, does not show as an overrun the fixture never declared.
func (c *setupClock) expired() *SetupTimeoutError {
	return &SetupTimeoutError{After: c.after}
}

// noteSetupBudget moves the budget-driven setup deadline for a fixture whose
// setup starts now with budget; zero lifts the deadline for good.
func (p *SharedFixtureProcess) noteSetupBudget(budget time.Duration) {
	p.mu.Lock()
	if budget <= 0 {
		p.setupUnbounded = true
	} else if at := time.Now().Add(budget); at.After(p.setupDeadline) {
		p.setupDeadline, p.setupBudget = at, budget
	}
	p.mu.Unlock()
	select {
	case p.budgetSeen <- struct{}{}:
	default:
	}
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
	if err := p.WaitSetup(ctx, timeout); err != nil {
		p.shutdown()
		return err
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
	select {
	case <-p.done:
		return
	default:
	}
	_ = p.tree.Interrupt()
	p.awaitExit(p.teardownBudget())
}

// awaitExit waits up to budget for the subprocess to exit on its own,
// force-killing the group when it does not, and reports whether force was
// needed and how long the process was given. A process still setting up
// reports its teardown budget on _done, as its teardown begins, and a start
// command's acknowledgement reports it once the fixtures it started came up:
// from then on it is given that budget.
func (p *SharedFixtureProcess) awaitExit(budget time.Duration) (forceKilled bool, allowed time.Duration) {
	began := time.Now()
	deadline := began.Add(budget)
	timer := time.NewTimer(budget)
	defer timer.Stop()
	reported := p.allDone
	select {
	case <-reported:
		reported = nil
	default:
	}
	for {
		select {
		case <-p.done:
			return false, deadline.Sub(began)
		case <-reported:
			reported = nil
			deadline = p.extendTo(deadline, timer)
		case <-p.teardownSeen:
			deadline = p.extendTo(deadline, timer)
		case <-timer.C:
			allowed = deadline.Sub(began).Round(time.Millisecond)
			fmt.Fprintf(os.Stderr, "WARN: shared fixture process did not exit within %v, forcing termination\n", allowed)
			_ = p.tree.Kill()
			<-p.done
			return true, allowed
		}
	}
}

// extendTo moves deadline, and timer with it, to the reported budget from
// now when that ends later.
func (p *SharedFixtureProcess) extendTo(deadline time.Time, timer *time.Timer) time.Time {
	at := time.Now().Add(p.teardownBudget())
	if !at.After(deadline) {
		return deadline
	}
	timer.Reset(time.Until(at))
	return at
}

// teardownBudget returns the budget the subprocess last reported, on its
// _done line or a command's acknowledgement. Until one arrived, a setup
// following the fixtures' budgets gets [unreportedTeardownBudget] and any
// other 30 seconds.
func (p *SharedFixtureProcess) teardownBudget() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	switch {
	case p.teardownTimeout == 0:
		return unreportedTeardownBudget
	case p.teardownTimeout < 0:
		return 30 * time.Second
	}
	return p.teardownTimeout
}

// WriteSuiteStateFile writes the state file one suite process reads: the
// keys the suite requires, under a name only that suite in that package has.
// Two packages may declare a suite of the same name with different fixtures.
func (p *SharedFixtureProcess) WriteSuiteStateFile(pkg, suite string, keys []string) (string, error) {
	return p.WriteStateFileForKeys(sanitizePkgName(pkg)+"_"+suite, keys)
}

// WriteStateFileForKeys writes a state file containing exactly the specified
// keys and returns its path. A key with no state is refused: the suite would
// hydrate a zero value and fail somewhere far from the cause.
func (p *SharedFixtureProcess) WriteStateFileForKeys(name string, keys []string) (string, error) {
	p.mu.Lock()
	subset := make(map[string]json.RawMessage, len(keys))
	for _, k := range keys {
		v, ok := p.state[k]
		if !ok {
			p.mu.Unlock()
			return "", fmt.Errorf("shared fixture %s has no state for %s: it was not started for this run", k, name)
		}
		subset[k] = v
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
// acknowledgement, ctx ending, or the deadline flat sets as for [WaitSetup]:
// at zero, the budgets the started fixtures declare.
func (p *SharedFixtureProcess) StartKeys(ctx context.Context, keys []string, flat time.Duration) error {
	return p.command(ctx, "start", keys, flat)
}

// TeardownKeys asks the setup subprocess to tear down the given fixtures now,
// in reverse dependency order — the releasing half of a window boundary.
// Teardown (the terminal owner) later skips keys already released here.
// Blocks until the acknowledgement, ctx ending, or timeout; timeout 0 or less
// means no deadline.
func (p *SharedFixtureProcess) TeardownKeys(ctx context.Context, keys []string, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = -1
	}
	return p.command(ctx, "teardown", keys, timeout)
}

// command sends one window verb and waits for the ack carrying its id. The
// process serves commands concurrently, so one that hangs holds up nothing
// sent after it.
func (p *SharedFixtureProcess) command(ctx context.Context, verb string, keys []string, flat time.Duration) error {
	if len(keys) == 0 {
		return nil
	}
	if p.stdin == nil {
		return fmt.Errorf("shared fixture process has no command channel")
	}
	resp := make(chan string, 1)
	p.cmdMu.Lock()
	p.cmdNext++
	id := p.cmdNext
	if p.cmdWaiters == nil {
		p.cmdWaiters = map[uint64]chan string{}
	}
	p.cmdWaiters[id] = resp
	p.cmdMu.Unlock()
	defer func() {
		p.cmdMu.Lock()
		delete(p.cmdWaiters, id)
		p.cmdMu.Unlock()
	}()

	line, err := json.Marshal(struct {
		Verb string   `json:"verb"`
		Keys []string `json:"keys"`
		ID   uint64   `json:"id"`
	}{Verb: verb, Keys: keys, ID: id})
	if err != nil {
		return err
	}
	if verb == "start" {
		p.mu.Lock()
		p.setupBegan, p.setupDeadline, p.setupBudget, p.setupUnbounded = time.Now(), time.Time{}, 0, false
		p.mu.Unlock()
	}
	p.cmdMu.Lock()
	_, err = p.stdin.Write(append(line, '\n'))
	p.cmdMu.Unlock()
	if err != nil {
		return fmt.Errorf("send %s to shared fixture process: %w", verb, err)
	}

	clock := p.startSetupClock(flat)
	defer clock.stop()
	for {
		select {
		case msg := <-resp:
			if msg != "" {
				return fmt.Errorf("shared fixture %s: %s", verb, msg)
			}
			return nil
		case <-p.done:
			return fmt.Errorf("shared fixture process exited before acknowledging %s", verb)
		case <-ctx.Done():
			return fmt.Errorf("shared fixture %s cancelled: %w", verb, ctx.Err())
		case <-clock.C:
			return fmt.Errorf("shared fixture %s timed out after %v", verb, clock.expired().After)
		case <-p.budgetSeen:
			clock.follow()
		}
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

	// Noted before signalling: a process already gone is not signalled again,
	// and an exit that predates the shutdown request happened while tests may
	// still have been running, a different story than dying mid-teardown.
	diedEarly := false
	select {
	case <-p.done:
		diedEarly = true
	default:
	}

	if !diedEarly {
		_ = p.tree.Interrupt()
	}
	forceKilled, allowed := p.awaitExit(budget)
	if p.sharedDir != "" {
		os.RemoveAll(p.sharedDir)
	}

	// A force-killed process was cut off mid-AfterAll. Its exit status says
	// nothing — a signalled process reports -1, never sharedTeardownFailedExit —
	// so without reporting it here the run would end green while the containers
	// the fixture was still stopping stay up. A setup failure earlier in the
	// run changes nothing about that.
	if forceKilled {
		return fmt.Errorf("shared fixture teardown was force-killed after %s; its AfterAll never finished and its resources may be leaked", allowed)
	}
	p.mu.Lock()
	protoErr := p.protoErr
	p.mu.Unlock()
	return errors.Join(exitVerdict(p.setupErr, p.waitErr, diedEarly), protoErr)
}

// exitVerdict judges the process's exit status once it exited unforced.
func exitVerdict(setupErr, waitErr error, diedEarly bool) error {

	// A setup failure already surfaced through SetupErr; the subprocess ran its
	// sibling teardowns under the runner's shutdown request and exited 1 for
	// the setup failure. Reporting that status here again would blame teardown
	// for it. Sibling AfterAll failures still reach the run through the
	// fixtures' own reports, which the pipeline reads separately.
	if setupErr != nil {
		return nil
	}

	// A clean exit is the only proof of a completed teardown: the epilogue runs
	// on any shutdown signal — whoever sent it — and exits 0 only when every
	// AfterAll passed. Everything else fails, with the most precise message the
	// status supports. exec maps a success status under a canceled command
	// context to ctx.Err() — and only a success status, so a context error here
	// IS the clean exit, just relabeled.
	if waitErr == nil || errors.Is(waitErr, context.Canceled) || errors.Is(waitErr, context.DeadlineExceeded) {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) && exitErr.ExitCode() == sharedTeardownFailedExit {
		return fmt.Errorf("shared fixture teardown failed")
	}
	if diedEarly {
		return fmt.Errorf("shared fixture process exited before teardown was requested (%v); its AfterAll may not have run and its resources may be leaked", waitErr)
	}
	return fmt.Errorf("shared fixture process died during teardown (%v); its AfterAll may not have finished and its resources may be leaked", waitErr)
}

// StartSharedFixtures generates a shared setup binary in the overlay temp dir,
// starts it as a subprocess, and returns a SharedFixtureProcess immediately.
// A background goroutine reads JSON state lines from stdout, closing per-fixture
// ready channels as each fixture completes. The caller must either wait on
// individual Ready() channels (streaming) or call WaitAllReady (batch).
// setupTimeout is stored as the initial teardownTimeout, which sets the
// force-kill window until the process reports its own (see teardownBudget).
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
	defer logSlowBuild(os.Stderr, "shared fixture setup binary", slowBuildThreshold())()
	buildCmd := exec.CommandContext(ctx, "go", "build", "-o", setupBin, setupFile)
	buildCmd.Stderr = os.Stderr
	buildMp := NewManagedProcess(buildCmd, ProcessConfig{Grace: GraceKill})
	if err := buildMp.Start(); err != nil {
		return nil, fmt.Errorf("build shared fixture setup: %w", err)
	}
	if err := buildMp.WaitWithGrace(ctx); err != nil {
		return nil, fmt.Errorf("build shared fixture setup: %w", err)
	}

	keys := make([]string, len(fixtures))
	for i := range fixtures {
		keys[i] = fixtures[i].PkgPath + "." + fixtures[i].Identifier
	}
	return launchSharedFixtures(exec.CommandContext(ctx, setupBin), sharedDir, keys, setupTimeout)
}

// launchSharedFixtures starts cmd as the shared fixture process serving the
// fixtures with the given state keys.
func launchSharedFixtures(cmd *exec.Cmd, sharedDir string, keys []string, setupTimeout time.Duration) (*SharedFixtureProcess, error) {
	// The teardown budget the process reports is the only kill timer: exec's
	// WaitDelay stays unset, since any bound set here would have to be chosen
	// before the budget is known, and one below it becomes the budget — a
	// fixture given minutes to stop its containers is killed part-way through,
	// and because a signalled process reports no meaningful exit status, the
	// run still says ok.
	tree := proctree.New(cmd)
	// The run's cancellation — Ctrl-C, --timeout — does not reach this
	// process: the suites are still stopping, and their AfterAlls still need
	// the fixtures. Teardown, after the last suite, is the one owner of its
	// shutdown.
	cmd.Cancel = func() error { return nil }

	// The protocol pipe is the runner's own, not exec's StdoutPipe: exec closes
	// that one as soon as Wait sees the exit, and Wait may not run before the
	// scan has finished, so a child the fixture left on the pipe would hold
	// the scan, and with it the teardown verdict, until it let go. Here the
	// scan is bounded by the drain delay past the exit instead.
	proto, protoW, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	// Stderr is read the same way, for the same reason, and passed on. The
	// process's stdout joins it: the protocol has a descriptor of its own,
	// so what a fixture prints is never read as protocol. Windows passes no
	// extra files, so there the protocol stays on stdout, which the process
	// takes away from fixture code before any of it runs.
	stderr, stderrW, err := os.Pipe()
	if err != nil {
		_ = proto.Close()
		_ = protoW.Close()
		return nil, err
	}
	cmd.Stderr = stderrW
	if runtime.GOOS == "windows" {
		cmd.Stdout = protoW
	} else {
		cmd.Stdout = stderrW
		cmd.ExtraFiles = []*os.File{protoW} // descriptor 3
		cmd.Env = append(os.Environ(), protocol.EnvSharedProtocolFD+"=3")
	}
	closePipes := func() {
		for _, f := range []*os.File{proto, protoW, stderr, stderrW} {
			_ = f.Close()
		}
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		closePipes()
		return nil, err
	}

	began := time.Now()
	if err := tree.Start(); err != nil {
		closePipes()
		return nil, fmt.Errorf("start shared fixture process: %w", err)
	}
	_ = protoW.Close()
	_ = stderrW.Close()

	// Build per-fixture readiness channels.
	ready := make(map[string]chan struct{}, len(keys))
	for _, key := range keys {
		ready[key] = make(chan struct{})
	}

	allDone := make(chan struct{})
	state := make(map[string]json.RawMessage)
	waitDone := make(chan struct{})

	proc := &SharedFixtureProcess{
		cmd:             cmd,
		tree:            tree,
		sharedDir:       sharedDir,
		done:            waitDone,
		teardownTimeout: setupTimeout,
		setupBegan:      began,
		ready:           ready,
		state:           state,
		allDone:         allDone,
		stdin:           stdin,
		budgetSeen:      make(chan struct{}, 1),
		teardownSeen:    make(chan struct{}, 1),
		failures:        newFailureLog(os.Stderr),
	}

	track(tree, liveTree{root: cmd.Process.Pid, done: waitDone})
	scanned := make(chan struct{})
	logged := make(chan struct{})
	go func() {
		proc.waitErr = cmd.Wait()
		untrack(tree)
		tree.Release()
		drainOutput(scanned, sharedFixtureDrainDelay, proto)
		drainOutput(logged, sharedFixtureDrainDelay, stderr)
		close(waitDone)
	}()
	go func() {
		defer close(logged)
		_, _ = io.Copy(proc.failures, stderr)
	}()

	go func() {
		defer close(scanned)
		proc.readProtocol(proto)
	}()

	return proc, nil
}

// readProtocol reads the protocol stream to its end: state lines, the _done
// sentinel and _cmd acknowledgements.
func (p *SharedFixtureProcess) readProtocol(stream io.Reader) {
	closedReady := make(map[string]bool, len(p.ready))
	doneSeen := false
	// Lines are read whole, however long a fixture's state is. The scan
	// outlives _done: window verbs keep the stream alive with late state
	// lines (deferred StartKeys) and _cmd acknowledgements, until EOF at
	// process exit.
	lines := bufio.NewReader(stream)
	var readErr error
	for {
		line, err := lines.ReadBytes('\n')
		if err != nil && len(line) == 0 {
			readErr = err
			break
		}
		var entry fixtureStateEntry
		if json.Unmarshal(line, &entry) != nil {
			continue
		}
		if entry.Key == "_done" {
			if doneSeen {
				// The stream is the process's alone now; a second sentinel is
				// its error, not the runner's to trip over.
				p.protocolError(fmt.Errorf("the shared fixture process reported _done twice"))
				continue
			}
			if entry.Error != "" {
				p.setupErr = fmt.Errorf("%s", entry.Error)
			}
			p.noteTeardownBudget(entry.TeardownBudget)
			doneSeen = true
			close(p.allDone)
			continue
		}
		if entry.Key == "_setup" {
			if d, err := time.ParseDuration(entry.Budget); err == nil {
				p.noteSetupBudget(d)
			}
			continue
		}
		if entry.Key == "_cmd" {
			// Before the ack: the command's waiter reads the budget next.
			if p.noteTeardownBudget(entry.TeardownBudget) {
				select {
				case p.teardownSeen <- struct{}{}:
				default:
				}
			}
			// An ack no one waits for any more (the command was given up)
			// is dropped.
			p.cmdMu.Lock()
			resp := p.cmdWaiters[entry.ID]
			p.cmdMu.Unlock()
			if resp != nil {
				resp <- entry.Error
			}
			continue
		}
		p.mu.Lock()
		p.state[entry.Key] = entry.State
		p.mu.Unlock()
		if ch, ok := p.ready[entry.Key]; ok && !closedReady[entry.Key] {
			close(ch)
			closedReady[entry.Key] = true
		}
	}
	if doneSeen {
		return
	}
	// A read end closed by the drain, like EOF, is the end of the stream too.
	if readErr != nil && !errors.Is(readErr, io.EOF) && !errors.Is(readErr, os.ErrClosed) {
		p.setupErr = fmt.Errorf("reading the protocol stream: %w", readErr)
	} else if p.setupErr == nil {
		p.setupErr = fmt.Errorf("subprocess exited without _done sentinel")
	}
	close(p.allDone)
}

// noteTeardownBudget takes a teardown budget the process reported and says
// whether there was one. Under p.mu: an interrupted run can reach Teardown
// while the stream is still being read.
func (p *SharedFixtureProcess) noteTeardownBudget(reported string) bool {
	d, err := time.ParseDuration(reported)
	if err != nil || d <= 0 {
		return false
	}
	p.mu.Lock()
	p.teardownTimeout = d
	p.mu.Unlock()
	return true
}

// protocolError records a stream the process got wrong after its setup was
// read as done: the setup's verdict stands, the teardown's adds this, and the
// failure log carries it.
func (p *SharedFixtureProcess) protocolError(err error) {
	p.mu.Lock()
	if p.protoErr == nil {
		p.protoErr = err
	}
	p.mu.Unlock()
	fmt.Fprintf(p.failures, "shared fixture protocol: %v\n", err)
}
