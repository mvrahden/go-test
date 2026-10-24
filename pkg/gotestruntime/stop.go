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
// that code; a parent that is gone counts as well. The stop file counts by
// itself too, which the watcher polls for: on Windows it is the only stop the
// runner sends. Under plain go test only SIGINT counts, the Ctrl-C a developer
// sends, and nothing is polled. Fuzzing is left to Go once its engine has the
// target: it stops on an interrupt by itself, saving what it found, and the
// target returns. A signal ignored when the process started stays ignored.
type watcher struct {
	stderr  io.Writer
	file    string // the runner's stop file; empty under plain go test
	sigs    []os.Signal
	ch      chan os.Signal // the signals that arrived
	found   chan os.Signal // a stop the poll found
	running bool
	// unasked: the stop under way is one the poll found, which no signal
	// has asked for yet.
	unasked bool
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

// stopFilePoll is how often the watcher looks for the runner's stop file.
const stopFilePoll = 25 * time.Millisecond

// poll hands the loop sig once the runner's stop file exists. It runs under
// the runner only, and fires once.
func (w *watcher) poll(sig os.Signal) {
	defer close(w.polled)
	tick := time.NewTicker(stopFilePoll)
	defer tick.Stop()
	for {
		select {
		case <-w.quit:
			return
		case <-tick.C:
		}
		if _, err := os.Stat(w.file); err == nil {
			w.found <- sig // buffered, and sent once
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
			// A stop a signal already began has read the file too.
			if !stopping.Load() && w.counts(sig) {
				w.stop(sig, false)
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
	return os.Getppid() != parentPID
}

// stopGrace is how long a stop waits for a running method body to return once
// its context has ended. A method still running then ignores its context, and
// nothing but ending the process can stop it.
const stopGrace = 5 * time.Second

// stop tells the run to stop and lets it finish on its own: the tests and the
// setups in flight see their contexts end, the methods not yet started are
// skipped, AfterAll runs, and the hold's cleanup releases the fixtures. A
// body or BeforeAll still running stopGrace after the stop, or after its own
// start when that came later, a second stop signal, or a run still going when
// only the fixtures' teardown time is left of the release bound, forces it:
// the fixtures are torn down within what is left of the bound and the process
// ends by sig. Once the stop is settled or forced, the handlers are reset, so
// the next signal has its default action. A stop no signal asked for, which
// the poll found, takes the first signal that follows as its request, the
// runner's own after its stop file among them; only the next one forces it.
func (w *watcher) stop(sig os.Signal, requested bool) {
	w.unasked = !requested
	releaseEnd, settled := w.settle(sig, stopGrace)
	signal.Reset(w.sigs...)
	if settled {
		return
	}
	w.force(sig, releaseEnd)
}

// force tears the held fixtures down, until releaseEnd, and ends the process
// by sig.
func (w *watcher) force(sig os.Signal, releaseEnd time.Time) {
	w.release(releaseEnd)
	reraise(sig)
}

// release tears the held fixtures down, until releaseEnd, and says so when
// that does not finish, naming the time the release got.
func (w *watcher) release(releaseEnd time.Time) (finished bool) {
	got := time.Until(releaseEnd)
	if finished = releaseWithin(releaseEnd); !finished {
		fmt.Fprintf(w.stderr, "gotest: fixture teardown did not finish within %s; what it holds may be left behind\n", got.Round(100*time.Millisecond))
	}
	return finished
}

// settle stops the run and waits for it to finish on its own, as [watcher.stop]
// describes. It reports whether the run finished, and until when a forced
// release may run.
func (w *watcher) settle(sig os.Signal, grace time.Duration) (releaseEnd time.Time, settled bool) {
	stoppedSig.Store(sig)
	stopping.Store(true)
	runstate.Stop()
	stopAt := time.Now()
	releaseEnd = stopAt.Add(releaseBound())
	// The fixtures' teardown keeps its share of the bound for a forced release.
	runEnd := releaseEnd.Add(-releaseReserve())
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for {
		now := time.Now()
		if kind := runstate.Overdue(stopAt, grace, now, settleExcept); kind != "" {
			fmt.Fprintf(w.stderr, "gotest: %s did not return within %s of its context ending; it is cut short, the fixtures are torn down and the process ends by the signal\n", kind, grace)
			return releaseEnd, false
		}
		if finished() {
			return releaseEnd, true
		}
		if now.After(runEnd) {
			fmt.Fprintln(w.stderr, "gotest: the run did not stop within its budget; the fixtures are torn down and the process ends by the signal")
			return releaseEnd, false
		}
		select {
		case again := <-w.ch:
			switch {
			case !w.counts(again):
			case w.unasked:
				w.unasked = false // the request the poll ran ahead of
			default:
				fmt.Fprintln(w.stderr, "gotest: a second stop signal; the fixtures are torn down now and the process ends by the signal")
				return releaseEnd, false
			}
		case <-tick.C:
		}
	}
}

// settleExcept is a test whose own body and suite a stop does not wait for:
// set only by tests that stop the run from inside it.
var settleExcept any

// finished reports whether the stopped run has ended on its own: no body,
// setup, hold or suite is left.
func finished() bool {
	if runstate.Bodies(settleExcept) > 0 {
		return false
	}
	setupMu.Lock()
	setups := setupInFlight
	setupMu.Unlock()
	heldMu.Lock()
	holds := len(held)
	heldMu.Unlock()
	return setups == 0 && holds == 0 && openSuites.Load() == 0
}

// stoppedSig is the signal the stop came by, for the process to end by.
var stoppedSig atomic.Value // os.Signal

func stoppedBy() os.Signal {
	if sig, ok := stoppedSig.Load().(os.Signal); ok {
		return sig
	}
	return os.Interrupt
}

// openSuites counts the suites opened and not yet torn down.
var openSuites atomic.Int64

// close stops the watcher.
func (w *watcher) close() {
	if len(w.sigs) > 0 {
		signal.Stop(w.ch)
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

// The teardown budget this process advertised to the runner: the longest
// fixture path plus suite setup plus headroom, over every DAG it set up.
var (
	budgetMu  sync.Mutex
	maxBudget time.Duration
	// maxReserve is the longest fixture teardown path among them.
	maxReserve time.Duration
)

// recordReserve records d, a DAG's longest teardown path.
func recordReserve(d time.Duration) {
	budgetMu.Lock()
	defer budgetMu.Unlock()
	if d > maxReserve {
		maxReserve = d
	}
}

// releaseReserve is the part of the release bound a forced release keeps
// for the fixtures' teardown, never more than the bound.
func releaseReserve() time.Duration {
	bound := releaseBound()
	budgetMu.Lock()
	r := maxReserve
	budgetMu.Unlock()
	return min(r, bound)
}

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
