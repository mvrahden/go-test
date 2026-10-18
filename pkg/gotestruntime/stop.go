package gotestruntime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/internal/runstate"
)

// The sidecar files the runner shares with this process, read once and taken
// out of the environment: a process a test starts must not answer to them.
var (
	stopFile   = takeEnv(protocol.EnvStopFile)
	budgetFile = takeEnv(protocol.EnvTeardownBudgetFile)
)

func takeEnv(key string) string {
	v := os.Getenv(key)
	_ = os.Unsetenv(key)
	return v
}

// parentPID is the process that started this one. Under the runner, a parent
// that is gone means the CLI crashed and nobody is left to announce a stop.
var parentPID = os.Getppid()

// The watcher runs while a suite does: the first suite or hold starts it, the
// one that ends last stops it, so it leaves no goroutine and no signal handler
// behind between them.
var (
	watchMu  sync.Mutex
	watching int
	active   *watcher
)

func startWatch() {
	watchMu.Lock()
	defer watchMu.Unlock()
	if watching == 0 {
		active = watch(os.Stderr)
	}
	watching++
}

func stopWatch() {
	watchMu.Lock()
	defer watchMu.Unlock()
	watching--
	if watching == 0 {
		active.close()
		active = nil
		// Under plain go test nothing else marks the run interrupted: once the
		// stopped suite has torn down, the process ends by the signal, and the
		// suites after it never run. Under the runner the process ends with the
		// tests' own status, so the stream carries every verdict and the runner
		// reports the stop.
		if stopFile == "" && stopping.Load() {
			reraise(stoppedBy())
		}
	}
}

// watcher turns a stop request into fixture teardown.
//
// Under the gotest runner a signal counts only once the runner has created the
// stop file, so a signal the code under test sends its own process is left to
// that code; a parent that is gone counts as well. Under plain go test only
// SIGINT counts, the Ctrl-C a developer sends. Fuzzing is left to Go once its
// engine has the target: it stops on an interrupt by itself, saving what it
// found, and the target returns. A signal ignored when the process started
// stays ignored.
type watcher struct {
	stderr io.Writer
	sigs   []os.Signal
	ch     chan os.Signal
	quit   chan struct{}
	done   chan struct{}
}

func watch(stderr io.Writer) *watcher {
	notePipe()
	w := &watcher{stderr: stderr, ch: make(chan os.Signal, 1), quit: make(chan struct{}), done: make(chan struct{})}
	candidates := []os.Signal{os.Interrupt}
	if stopFile != "" {
		candidates = append(candidates, stopSignals...)
		// The runner signals this process by pid, so the converter in front
		// of it lives on to relay the stream to its end.
		_ = os.WriteFile(protocol.StopPIDPath(stopFile), []byte(strconv.Itoa(os.Getpid())), 0o600)
	}
	for _, s := range candidates {
		if !signal.Ignored(s) {
			w.sigs = append(w.sigs, s)
		}
	}
	if len(w.sigs) == 0 {
		close(w.done)
		return w
	}
	signal.Notify(w.ch, w.sigs...)
	go w.loop()
	return w
}

func (w *watcher) loop() {
	defer close(w.done)
	for {
		select {
		case <-w.quit:
			return
		case sig := <-w.ch:
			if w.counts(sig) {
				w.stop(sig) // returns once the run has stopped, or ends the process
			}
		}
	}
}

func (w *watcher) counts(sig os.Signal) bool {
	if runstate.FuzzWorker() {
		return false
	}
	if runstate.FuzzTarget() != "" && runstate.FuzzEngine() {
		return false
	}
	if stopFile == "" {
		return sig == os.Interrupt
	}
	if _, err := os.Stat(stopFile); err == nil {
		return true
	}
	return os.Getppid() != parentPID
}

// stopGrace is how long a stop waits for a running method to return once its
// context has ended. A method still running then ignores its context, and
// nothing but ending the process can stop it.
const stopGrace = 5 * time.Second

// stop tells the run to stop and lets it finish on its own: the tests and the
// setups in flight see their contexts end, the methods not yet started are
// skipped, AfterAll runs, and the hold's cleanup releases the fixtures, so the
// stream carries every verdict. A method that does not return within
// stopGrace, or a run that does not finish within the release bound, is
// forced: the fixtures are torn down, within that same bound, and the process
// ends by sig.
func (w *watcher) stop(sig os.Signal) {
	signal.Reset(w.sigs...)
	stoppedSig.Store(sig)
	stopping.Store(true)
	runstate.Stop()
	deadline := time.Now().Add(releaseBound())
	grace := time.Now().Add(stopGrace)
	if grace.After(deadline) {
		grace = deadline
	}
	switch {
	case !waitBodiesEnd(grace):
		fmt.Fprintf(w.stderr, "gotest: a method did not return within %s of its context ending; it is cut short, the fixtures are torn down and the process ends by the signal\n", stopGrace)
	case waitSetups(deadline) && waitHoldsEnd(deadline):
		return
	default:
		fmt.Fprintln(w.stderr, "gotest: the run did not stop within its budget; the fixtures are torn down and the process ends by the signal")
	}
	if !releaseWithin(deadline) {
		fmt.Fprintf(w.stderr, "gotest: fixture teardown did not finish within %s; what it holds may be left behind\n", releaseBound())
	}
	reraise(sig)
}

// stoppedSig is the signal the stop came by, for the process to end by.
var stoppedSig atomic.Value // os.Signal

func stoppedBy() os.Signal {
	if sig, ok := stoppedSig.Load().(os.Signal); ok {
		return sig
	}
	return os.Interrupt
}

// waitBodiesEnd waits until no test method is running, until deadline.
func waitBodiesEnd(deadline time.Time) bool {
	for {
		running := 0
		bodies.Range(func(any, any) bool { running++; return true })
		if running == 0 {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// waitHoldsEnd waits until no fixtures are held, until deadline: the hold's
// own cleanup drops them once the suite's AfterAll has run.
func waitHoldsEnd(deadline time.Time) bool {
	for {
		heldMu.Lock()
		n := len(held)
		heldMu.Unlock()
		if n == 0 {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// close stops the watcher.
func (w *watcher) close() {
	if len(w.sigs) > 0 {
		signal.Stop(w.ch)
		close(w.quit)
	}
	<-w.done
}

// releaseWithin tears the held fixtures down, giving up waiting at deadline.
func releaseWithin(deadline time.Time) (finished bool) {
	done := make(chan struct{})
	go func() {
		releaseHeld()
		close(done)
	}()
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	select {
	case <-done:
		return true
	case <-timer.C:
		return false
	}
}

// stopping is set once a stop begins; no fixture is set up after it.
var stopping atomic.Bool

// Fixture setup in flight. A stop waits for it: a cancelled setup tears down
// what it brought up.
var (
	setupMu       sync.Mutex
	setupInFlight int
)

// beginSetup admits a fixture setup unless a stop has begun, and returns the
// context it runs under: canceled by a stop as well as by ctx.
func beginSetup(ctx context.Context) (context.Context, func(), error) {
	setupMu.Lock()
	defer setupMu.Unlock()
	if stopping.Load() {
		return nil, nil, errors.New("the run is stopping: no fixture is set up any more")
	}
	setupInFlight++
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(runstate.StopContext(), cancel)
	return ctx, func() {
		stop()
		cancel()
		setupMu.Lock()
		setupInFlight--
		setupMu.Unlock()
	}, nil
}

// waitSetups waits for setups in flight, until deadline.
func waitSetups(deadline time.Time) bool {
	for {
		setupMu.Lock()
		n := setupInFlight
		setupMu.Unlock()
		if n == 0 {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// The teardown budget this process advertised to the runner: the longest
// fixture path plus suite setup plus headroom, over every DAG it set up.
var (
	budgetMu  sync.Mutex
	maxBudget time.Duration
)

func recordBudget(d time.Duration) time.Duration {
	budgetMu.Lock()
	defer budgetMu.Unlock()
	if d > maxBudget {
		maxBudget = d
	}
	return maxBudget
}

// releaseBound is how long a stop may spend releasing fixtures: within the
// budget the runner waits before it kills the process, so the process says
// what it could not release instead of being killed silently.
func releaseBound() time.Duration {
	budgetMu.Lock()
	b := maxBudget
	budgetMu.Unlock()
	if b <= 0 {
		return 2 * time.Minute
	}
	if b > 10*time.Second {
		return b - 5*time.Second
	}
	return b
}
