package gotestruntime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mvrahden/go-test/internal/control"
	"github.com/mvrahden/go-test/internal/runstate"
)

// link is the channel the runner passed this process, taken out of the
// environment; nil under plain go test.
var link = claimLink()

// claimLink takes the runner's channel and says so at once, not at the first
// suite's open: the runner falls back to a signal when no hello comes soon
// after the start, and a TestMain may work long before m.Run. A stop sent
// meanwhile waits on the channel for the first open.
func claimLink() *control.Endpoint {
	ep := control.Claim()
	if ep != nil {
		_ = ep.Hello(os.Getpid())
	}
	return ep
}

// legacyRunner: a gotest CLI older than this runtime started the process. It
// names a stop file and passes no channel, and its stop would kill the suite
// without teardown, so no suite runs under it.
var legacyRunner = takeLegacyStopFile() && link == nil

// takeLegacyStopFile reports whether an older CLI named its stop file, and
// takes the variable out of the environment.
func takeLegacyStopFile() bool {
	_, ok := os.LookupEnv("GOTEST_STOP_FILE")
	_ = os.Unsetenv("GOTEST_STOP_FILE")
	return ok
}

// refuseLegacyRunner fails tb under an older CLI, naming the fix.
func refuseLegacyRunner(tb testing.TB) {
	if !legacyRunner {
		return
	}
	tb.Helper()
	bi, ok := debug.ReadBuildInfo()
	tb.Fatal(legacyRefusal(bi, ok))
}

const runtimeModule = "github.com/mvrahden/go-test"

// legacyRefusal says this runtime, at the version bi names, is newer than the
// CLI running it, and how to run one that matches.
func legacyRefusal(bi *debug.BuildInfo, ok bool) string {
	version, pin := "this tree", "<the version go.mod requires>"
	if ok {
		for _, m := range append([]*debug.Module{&bi.Main}, bi.Deps...) {
			if m.Path != runtimeModule {
				continue
			}
			// A replaced module is a tree of its own, of no version to pin.
			if m.Replace == nil && m.Version != "" && m.Version != "(devel)" {
				version, pin = runtimeModule+" "+m.Version, m.Version
			}
			break
		}
	}
	return "gotest: this runtime (" + version + ") is newer than the gotest CLI running it, which cannot stop it; run go tool gotest, or go get -tool " + runtimeModule + "/cmd/gotest@" + pin
}

// The watcher runs while a suite does: the first suite or hold starts it, the
// one that ends last stops it, so it leaves no goroutine and no signal handler
// behind between them.
var (
	watchMu  sync.Mutex
	watching int
	active   *watcher
	// unread holds what a closed watcher's reader took from the channel and
	// could not hand over; the next open acts on it.
	unread []control.Line
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
		w := active
		w.close()
		active = nil
		unread = append(unread, w.unread...)
		// Under plain go test nothing else marks the run interrupted: once the
		// stopped suite has torn down, the process ends by the signal, and the
		// suites after it never run. Under the runner the process ends with the
		// tests' own status, so the stream carries every verdict and the runner
		// reports the stop; a stop a signal began on the runner's channel ends
		// by the signal, as under plain go test. With the runner gone, the
		// process group ends with it.
		switch {
		case runnerGone.Load():
			endGroup()
		case stopping.Load() && (!w.underRunner() || bySignal.Load()):
			reraise(stoppedBy())
		}
	}
}

// drainLink acts, at a suite's open, on what reached the runner's channel
// while no suite read it: during TestMain, or between two suites. A stop, a
// force or the channel's end stops the run before anything is set up; with
// nothing open there is nothing to settle, so the process ends with the
// tests' own status, and a signal that follows changes nothing. While a
// watcher runs, its reader takes the lines.
func drainLink() {
	if link == nil {
		return
	}
	watchMu.Lock()
	defer watchMu.Unlock()
	if active != nil {
		return
	}
	lines, gone := link.Drain()
	lines = append(lines, unread...)
	unread = nil
	if leftToGo() {
		return
	}
	asked := false
	for _, l := range lines {
		switch l.Kind {
		case control.Stop, control.Force:
			asked = true
		case endOfChannel:
			gone = true
		}
	}
	if gone {
		runnerGone.Store(true)
	}
	if stopping.Load() || !asked && !gone {
		return
	}
	if !asked {
		_ = link.Stopped("runner-gone")
	}
	stoppedSig.Store(runnerSignal())
	stopping.Store(true)
	runstate.Stop()
	if sigs := caughtSignals(true); len(sigs) > 0 {
		signal.Notify(make(chan os.Signal, 1), sigs...)
	}
}

// watcher turns a stop request into fixture teardown.
//
// On the gotest runner's channel the runner's stop line asks for a stop, its
// force line forces one, and the channel's end, the runner gone, starts one
// nobody asked for; a signal starts a stop the runner is told of, and never
// forces one. Under plain go test only SIGINT counts, the Ctrl-C a developer
// sends, and a second one forces the stop. Fuzzing is left to Go once its
// engine has the target: it stops on an interrupt by itself, saving what it
// found, and the target returns. A signal ignored when the process started
// stays ignored.
type watcher struct {
	stderr  io.Writer
	link    *control.Endpoint // the runner's channel; nil without one
	sigs    []os.Signal
	ch      chan os.Signal    // the signals that arrived
	lines   chan control.Line // the runner's lines, then endOfChannel
	running bool
	// forced: the runner's force line came, so the stop releases at once.
	forced   bool
	quit     chan struct{}
	done     chan struct{}
	readDone chan struct{}
	unread   []control.Line // taken from the channel, not handed over by quit
}

// endOfChannel is the kind the watcher gives the end of the runner's
// channel; no line has it.
const endOfChannel control.Kind = 0

func watch(stderr io.Writer) *watcher { return watchOn(stderr, link) }

func watchOn(stderr io.Writer, ep *control.Endpoint) *watcher {
	notePipe()
	w := &watcher{stderr: stderr, link: ep, ch: make(chan os.Signal, 1),
		lines: make(chan control.Line), quit: make(chan struct{}), done: make(chan struct{}),
		readDone: make(chan struct{})}
	w.sigs = caughtSignals(w.underRunner())
	if len(w.sigs) == 0 && !w.underRunner() {
		close(w.done)
		close(w.readDone)
		return w
	}
	if len(w.sigs) > 0 {
		signal.Notify(w.ch, w.sigs...)
	}
	w.running = true
	go w.loop()
	if w.link != nil {
		go w.read()
	} else {
		close(w.readDone)
	}
	return w
}

// underRunner reports whether the gotest runner started this watcher's
// process, which then has its channel.
func (w *watcher) underRunner() bool { return w.link != nil }

// caughtSignals is what a watcher catches: Ctrl-C's signal, and under the
// runner its stop signals too, leaving out any ignored at the start.
func caughtSignals(underRunner bool) []os.Signal {
	candidates := []os.Signal{os.Interrupt}
	if underRunner {
		candidates = append(candidates, stopSignals...)
	}
	var sigs []os.Signal
	for _, s := range candidates {
		if !signal.Ignored(s) {
			sigs = append(sigs, s)
		}
	}
	return sigs
}

// runnerSignal is the signal a stop the runner began ends the process by when
// it is forced: the runner's own stop signal.
func runnerSignal() os.Signal {
	if len(stopSignals) > 0 {
		return stopSignals[0]
	}
	return os.Interrupt
}

// signalName spells a signal the watcher catches as the runner's stopped line
// does: Ctrl-C's, or else the runner's stop signal, the only other one.
func signalName(sig os.Signal) string {
	if sig == os.Interrupt {
		return "SIGINT"
	}
	return "SIGTERM"
}

// read hands the loop what the runner sends on its channel, then the
// channel's end, until quit closes. A line taken and not handed over by then
// is kept for the next suite's open.
func (w *watcher) read() {
	defer close(w.readDone)
	for {
		l, err := w.link.Next(w.quit)
		if errors.Is(err, control.ErrStopped) {
			return
		}
		if err != nil {
			l = control.Line{Kind: endOfChannel}
		}
		select {
		case w.lines <- l:
		case <-w.quit:
			w.unread = append(w.unread, l)
			return
		}
		if err != nil {
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
				w.announce(sig)
				w.stop(sig) // returns once the run has stopped, or ends the process
			}
		case l := <-w.lines:
			w.take(l)
		}
	}
}

// take starts the stop a line asks for while none is under way: the stop line
// a requested one, the force line one forced from the start, and the
// channel's end one nobody asked for, which the runner is told of in case it
// lives. After a stop a line changes nothing, but the channel's end still
// marks the runner gone.
func (w *watcher) take(l control.Line) {
	if leftToGo() {
		return
	}
	if l.Kind == endOfChannel {
		runnerGone.Store(true)
	}
	if stopping.Load() {
		return
	}
	switch l.Kind {
	case control.Stop:
		w.stop(runnerSignal())
	case control.Force:
		w.forced = true
		w.stop(runnerSignal())
	case endOfChannel:
		_ = w.link.Stopped("runner-gone")
		w.stop(runnerSignal())
	}
}

// heed takes a line that arrives while a stop is under way: the force line
// forces it, and the channel's end marks the runner gone.
func (w *watcher) heed(l control.Line) {
	if leftToGo() {
		return
	}
	switch l.Kind {
	case control.Force:
		w.forced = true
	case endOfChannel:
		runnerGone.Store(true)
	}
}

// announce tells the runner, on its channel, of a stop a signal began, which
// it did not send; that stop ends the process by the signal.
func (w *watcher) announce(sig os.Signal) {
	if w.link == nil {
		return
	}
	bySignal.Store(true)
	_ = w.link.Stopped("signal:" + signalName(sig))
}

// signalsStopOnTheChannel: on the runner's channel, a signal landing while no
// stop is under way or done starts one, of which the runner is told; false
// drops every signal there.
const signalsStopOnTheChannel = true

// Set once the stop under way came by a signal on the runner's channel, and
// once that channel ended.
var bySignal, runnerGone atomic.Bool

func (w *watcher) counts(sig os.Signal) bool {
	if leftToGo() {
		return false
	}
	if w.link != nil {
		// Only the first signal: the runner forces a stop by its force line,
		// never by a signal.
		return signalsStopOnTheChannel && !stopping.Load()
	}
	return sig == os.Interrupt
}

// leftToGo reports whether a stop is Go's own to handle: in a fuzzing worker,
// and once the engine has the fuzz target.
func leftToGo() bool {
	return runstate.FuzzWorker() || runstate.FuzzTarget() != "" && runstate.FuzzEngine()
}

// stopGrace is how long a stop waits for a running method body to return once
// its context has ended. A method still running then ignores its context, and
// nothing but ending the process can stop it.
const stopGrace = 5 * time.Second

// stop tells the run to stop and lets it finish on its own: the tests and the
// setups in flight see their contexts end, the methods not yet started are
// skipped, AfterAll runs, and the hold's cleanup releases the fixtures. A
// body or BeforeAll still running stopGrace after the stop, or after its own
// start when that came later, a second Ctrl-C under plain go test or the
// runner's force line, or a run still going when only the fixtures' teardown
// time is left of the release bound, forces it: the fixtures are torn down
// within what is left of the bound and the process ends by sig. Once the stop
// is forced, or settled under plain go test, the handlers are reset, so the
// next signal has its default action; settled under the runner, the stop
// signals stay caught and dropped, as a process that ended its run is not one
// to end by a signal that comes late.
func (w *watcher) stop(sig os.Signal) {
	releaseEnd, settled := w.settle(sig, stopGrace)
	if settled && w.underRunner() {
		w.absorb()
		return
	}
	signal.Reset(w.sigs...)
	if settled {
		return
	}
	w.force(sig, releaseEnd)
}

// absorb keeps the stop signals caught and dropped for the rest of the
// process. Caught rather than ignored, so a process a test starts after it
// keeps the default.
func (w *watcher) absorb() {
	if len(w.sigs) > 0 { // none would catch every signal
		signal.Notify(make(chan os.Signal, 1), w.sigs...)
	}
}

// force tears the held fixtures down, until releaseEnd, and ends the process
// by sig, or, with the runner gone, ends its process group.
func (w *watcher) force(sig os.Signal, releaseEnd time.Time) {
	w.release(releaseEnd)
	if runnerGone.Load() {
		endGroup()
	}
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
		if w.forced {
			fmt.Fprintln(w.stderr, "gotest: the runner forced the stop; the fixtures are torn down now and the process ends by the signal")
			return releaseEnd, false
		}
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
			if w.counts(again) {
				fmt.Fprintln(w.stderr, "gotest: a second stop signal; the fixtures are torn down now and the process ends by the signal")
				return releaseEnd, false
			}
		case l := <-w.lines:
			w.heed(l)
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
		// Closed by the last suite's end, under a stop the run is settling.
		if w.underRunner() && stopping.Load() {
			w.absorb()
		}
		signal.Stop(w.ch)
	}
	if w.running {
		close(w.quit)
	}
	<-w.done
	<-w.readDone
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
