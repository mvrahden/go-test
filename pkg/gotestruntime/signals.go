package gotestruntime

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"time"

	"github.com/mvrahden/go-test/internal/runstate"
)

// parentPID is the process that started this one. Under the runner, a parent
// that is gone means the CLI crashed and nobody is left to announce a stop.
var parentPID = os.Getppid()

// watcher turns a stop request into fixture teardown. It lives exactly as long
// as runTests: started before m.Run, stopped once the fixtures are down, and
// leaves no goroutine and no signal handler behind.
//
// Under the gotest runner a signal counts only once the runner has created the
// stop file, so a signal the code under test sends its own process is left to
// that code; a parent that is gone counts as well. Under plain go test only
// SIGINT counts, the Ctrl-C a developer sends. Fuzzing is left to Go once its
// engine has the target: it stops on an interrupt by itself, saving what it
// found, and m.Run returns. A signal ignored when the process started stays
// ignored.
type watcher struct {
	stderr io.Writer
	sigs   []os.Signal
	ch     chan os.Signal
	quit   chan struct{}
	done   chan struct{}

	mu   sync.Mutex
	late os.Signal // arrived while Main tore down; re-raised after
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
			if !w.counts(sig) {
				continue
			}
			if beginStop() {
				w.stop(sig) // does not return
			}
			// Main is tearing down after the tests: let it finish, end by the
			// signal afterwards; a second one has Go's default action again.
			signal.Reset(w.sigs...)
			w.mu.Lock()
			w.late = sig
			w.mu.Unlock()
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

// stop releases the fixtures and ends the process by sig. The tests and the
// setups in flight are told first — their contexts derive from the stop — so
// work that honours its context lets go of what teardown needs.
func (w *watcher) stop(sig os.Signal) {
	signal.Reset(w.sigs...)
	runstate.Stop()
	deadline := time.Now().Add(releaseBound())
	if !waitSetups(deadline) {
		fmt.Fprintln(w.stderr, "gotest: fixture setup still running at the stop; what it brings up may be left behind")
	}
	if _, finished := teardownWithin(deadline); !finished {
		fmt.Fprintf(w.stderr, "gotest: fixture teardown did not finish within %s; what it holds may be left behind\n", releaseBound())
	}
	reraise(sig)
}

// close stops the watcher once Main tore the fixtures down, and ends the
// process by a signal that arrived meanwhile.
func (w *watcher) close() {
	if len(w.sigs) > 0 {
		signal.Stop(w.ch)
		close(w.quit)
	}
	<-w.done
	w.mu.Lock()
	late := w.late
	w.mu.Unlock()
	if late != nil {
		reraise(late)
	}
}

func flagDuration(name string) time.Duration {
	f := flag.Lookup(name)
	if f == nil {
		return 0
	}
	d, err := time.ParseDuration(f.Value.String())
	if err != nil {
		return 0
	}
	return d
}

func runtimeStack(buf []byte) int { return runtime.Stack(buf, true) }
