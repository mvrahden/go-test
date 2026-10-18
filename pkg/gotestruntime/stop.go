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
// that code. The stop file counts by itself too, which the watcher polls for:
// on Windows it is the only stop the runner sends. So does a parent that is
// gone, and a stream nobody reads any more, which the watcher checks for
// itself. Under plain go test only SIGINT counts, the Ctrl-C a developer
// sends, and nothing is polled. Fuzzing is left to Go once its engine has the
// target: it stops on an interrupt by itself, saving what it found, and the
// target returns. A signal ignored when the process started stays ignored.
type watcher struct {
	stderr  io.Writer
	file    string // the runner's stop file; empty under plain go test
	sigs    []os.Signal
	ch      chan os.Signal // the signals that arrived
	death   chan os.Signal // the death signal, which only wakes the watcher
	found   chan os.Signal // a stop the poll found
	running bool
	quit    chan struct{}
	done    chan struct{}
	polled  chan struct{}
}

func watch(stderr io.Writer) *watcher { return watchFile(stderr, stopFile) }

func watchFile(stderr io.Writer, file string) *watcher {
	notePipe()
	w := &watcher{stderr: stderr, file: file, ch: make(chan os.Signal, 1), found: make(chan os.Signal, 1),
		quit: make(chan struct{}), done: make(chan struct{}), polled: make(chan struct{})}
	candidates := []os.Signal{os.Interrupt}
	if w.file != "" {
		candidates = append(candidates, stopSignals...)
		// The runner stops this process alone, by pid or by the stop file, so
		// the converter in front of it lives on to relay the stream to its end.
		_ = os.WriteFile(protocol.StopPIDPath(w.file), []byte(strconv.Itoa(os.Getpid())), 0o600)
	}
	for _, s := range candidates {
		if !signal.Ignored(s) {
			w.sigs = append(w.sigs, s)
		}
	}
	if len(w.sigs) == 0 && w.file == "" {
		close(w.done)
		close(w.polled)
		return w
	}
	if len(w.sigs) > 0 {
		signal.Notify(w.ch, w.sigs...)
	}
	if w.file != "" && deathSignal != nil && !signal.Ignored(deathSignal) {
		w.death = make(chan os.Signal, 1)
		signal.Notify(w.death, deathSignal)
	}
	w.running = true
	go w.loop()
	if w.file != "" {
		sig := os.Interrupt
		if len(stopSignals) > 0 {
			sig = stopSignals[0]
		}
		go w.poll(sig)
	} else {
		close(w.polled)
	}
	return w
}

// stopFilePoll is how often the watcher looks for the runner's stop file;
// readerPoll how often it checks that its parent lives and its stream is
// still read.
const (
	stopFilePoll = 25 * time.Millisecond
	readerPoll   = 200 * time.Millisecond
)

// poll hands the loop sig once the runner's stop file exists, the parent is
// gone, or nobody reads the stream any more: under -json behind an -exec
// wrapper that stays, the converter's death is the only trace a dead runner
// leaves. It runs under the runner only, and fires once.
func (w *watcher) poll(sig os.Signal) {
	defer close(w.polled)
	tick := time.NewTicker(stopFilePoll)
	defer tick.Stop()
	for n := 0; ; n++ {
		select {
		case <-w.quit:
			return
		case <-tick.C:
		}
		_, err := os.Stat(w.file)
		if err == nil || n%int(readerPoll/stopFilePoll) == 0 && (parentExited() || stdoutReaderGone()) {
			select {
			case w.found <- sig:
			default:
			}
			return
		}
	}
}

func (w *watcher) loop() {
	defer close(w.done)
	for {
		select {
		case <-w.quit:
			return
		case sig := <-w.ch:
			if w.counts(sig) {
				w.stop(sig, true) // returns once the run has stopped, or ends the process
			}
		case sig := <-w.found:
			if w.counts(sig) {
				w.stop(sig, false)
			}
		case <-w.death:
			// Only a parent that is gone makes it a stop, and the stop it
			// starts is nobody's request. Landing while a stop runs, it is
			// not read at all.
			if parentExited() {
				w.stop(stopSignals[0], false)
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
	if w.file == "" {
		return sig == os.Interrupt
	}
	if _, err := os.Stat(w.file); err == nil {
		return true
	}
	return os.Getppid() != parentPID || parentExited() || stdoutReaderGone()
}

// stopGrace is how long a stop waits for a running method body to return once
// its context has ended. A method still running then ignores its context, and
// nothing but ending the process can stop it.
const stopGrace = 5 * time.Second

// stop tells the run to stop and lets it finish on its own: the tests and the
// setups in flight see their contexts end, the methods not yet started are
// skipped, AfterAll runs, and the hold's cleanup releases the fixtures, so the
// stream carries every verdict. A method body that does not return within
// stopGrace, or a run that does not finish within the release bound, is
// forced: the fixtures are torn down, within that same bound, and the process
// ends by sig. A stop asked for by a signal resets the handlers, so the next
// signal ends the process. One no signal asked for, which the poll found or
// the death signal brought, keeps them for the first signal that follows, the
// runner's own after its stop file among them: that one is the first request,
// and only the next ends the process. The death signal is never a request.
func (w *watcher) stop(sig os.Signal, requested bool) {
	if len(w.sigs) > 0 {
		if requested {
			signal.Reset(w.sigs...)
		} else {
			done := make(chan struct{})
			defer close(done)
			go func() {
				select {
				case <-w.ch:
					signal.Reset(w.sigs...)
				case <-done:
				}
			}()
		}
	}
	deadline, settled := w.settle(sig, stopGrace)
	if settled {
		return
	}
	if !releaseWithin(deadline) {
		fmt.Fprintf(w.stderr, "gotest: fixture teardown did not finish within %s; what it holds may be left behind\n", releaseBound())
	}
	reraise(sig)
}

// settle stops the run and waits for it to finish on its own, as [stop]
// describes, giving a running method grace to return. It reports whether
// the run finished, and the deadline a forced release must keep.
func (w *watcher) settle(sig os.Signal, grace time.Duration) (deadline time.Time, settled bool) {
	stoppedSig.Store(sig)
	stopping.Store(true)
	runstate.Stop()
	deadline = time.Now().Add(releaseBound())
	graceEnd := time.Now().Add(grace)
	if graceEnd.After(deadline) {
		graceEnd = deadline
	}
	switch {
	case !waitBodiesEnd(graceEnd):
		fmt.Fprintf(w.stderr, "gotest: a method did not return within %s of its context ending; it is cut short, the fixtures are torn down and the process ends by the signal\n", grace)
	case waitSetups(deadline) && waitHoldsEnd(deadline):
		return deadline, true
	default:
		fmt.Fprintln(w.stderr, "gotest: the run did not stop within its budget; the fixtures are torn down and the process ends by the signal")
	}
	return deadline, false
}

// stoppedSig is the signal the stop came by, for the process to end by.
var stoppedSig atomic.Value // os.Signal

func stoppedBy() os.Signal {
	if sig, ok := stoppedSig.Load().(os.Signal); ok {
		return sig
	}
	return os.Interrupt
}

// waitBodiesEnd waits until no test-method body is running, until deadline.
// The hooks around a body are not waited for here: they run on within the
// release bound, so AfterEach and AfterAll finish before the fixtures go.
func waitBodiesEnd(deadline time.Time) bool {
	for {
		if runningBodies.Load() == 0 {
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
	}
	if w.death != nil {
		signal.Stop(w.death)
	}
	if w.running {
		close(w.quit)
	}
	<-w.done
	<-w.polled
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
