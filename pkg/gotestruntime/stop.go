package gotestruntime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
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

// The watcher runs while a hold does: the first hold starts it, the hold
// that ends last stops it, so it leaves no goroutine and no signal handler
// behind between holds.
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
				w.stop(sig) // does not return
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

// stop releases the held fixtures and ends the process by sig. The tests and
// the setups in flight are told first — their contexts derive from the stop —
// so work that honours its context lets go of what teardown needs. A hold
// already tearing down in its cleanup is waited for, not repeated.
func (w *watcher) stop(sig os.Signal) {
	signal.Reset(w.sigs...)
	stopping.Store(true)
	runstate.Stop()
	deadline := time.Now().Add(releaseBound())
	if !waitSetups(deadline) {
		fmt.Fprintln(w.stderr, "gotest: fixture setup still running at the stop; what it brings up may be left behind")
	}
	if !releaseWithin(deadline) {
		fmt.Fprintf(w.stderr, "gotest: fixture teardown did not finish within %s; what it holds may be left behind\n", releaseBound())
	}
	reraise(sig)
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
