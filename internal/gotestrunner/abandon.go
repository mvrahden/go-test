package gotestrunner

import (
	"context"
	"sync"
	"time"

	"github.com/mvrahden/go-test/internal/proctree"
	"github.com/mvrahden/go-test/internal/protocol"
)

// liveTree is a process tree running now, for a CLI that gives up on it.
type liveTree struct {
	root int // the root's pid, which leads the tree's process group on Unix
	// stopFile is the suite's stop file, next to which the suite process
	// publishes its pid; empty for a tree that runs no suite.
	stopFile string
	done     <-chan struct{} // closed once the tree is released; nil if unknown
}

var (
	liveMu sync.Mutex
	live   = map[*proctree.Tree]liveTree{}
)

func track(tree *proctree.Tree, l liveTree) {
	liveMu.Lock()
	defer liveMu.Unlock()
	live[tree] = l
}

func untrack(tree *proctree.Tree) {
	liveMu.Lock()
	defer liveMu.Unlock()
	delete(live, tree)
}

// Suite runs whose events are yet to reach the stream: a suite's events go
// out once its result is recorded, after its process ended.
var (
	relayMu   sync.Mutex
	relaying  int
	relayIdle = closed()
)

func closed() chan struct{} {
	c := make(chan struct{})
	close(c)
	return c
}

// relay counts a suite run until done marks its events passed on.
func relay() (done func()) {
	relayMu.Lock()
	if relaying == 0 {
		relayIdle = make(chan struct{})
	}
	relaying++
	relayMu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			relayMu.Lock()
			defer relayMu.Unlock()
			if relaying--; relaying == 0 {
				close(relayIdle)
			}
		})
	}
}

func relaysIdle() <-chan struct{} {
	relayMu.Lock()
	defer relayMu.Unlock()
	return relayIdle
}

// relayGrace is how long a tree killed at the bound gets for its suite's
// events to reach the stream.
const relayGrace = 500 * time.Millisecond

// runRecorded runs a suite and records its result, counted as a relay so
// a CLI that abandons the run still passes the suite's events on.
func runRecorded(ctx context.Context, c *OutputCollector, pkg string, idx int, t SuiteTarget, env []string, test2json bool) { //nolint:gocritic // hugeParam: as RunSingleSuite
	defer relay()()
	c.RecordResult(pkg, idx, RunSingleSuite(ctx, t, env, test2json))
}

// Abandon gives up on every process tree still running, for a CLI about to
// exit without waiting for them. Each suite process that published its pid
// is sent a second stop signal, which releases its fixtures at once, and a
// tree with no stop file, the shared fixture's, is asked to tear down; a tree
// neither reaches is killed at once. Within bound of that, or as soon as
// hurry closes, every tree still running is killed, so no process outlives
// the CLI. The events of the suites that ended are passed on before it
// returns, within bound, or relayGrace past it for a tree killed there.
func Abandon(bound time.Duration, hurry <-chan struct{}) {
	liveMu.Lock()
	trees := make(map[*proctree.Tree]liveTree, len(live))
	for tree, l := range live {
		trees[tree] = l
	}
	liveMu.Unlock()
	var stopping []<-chan struct{}
	for tree, l := range trees {
		if l.done != nil && askAgain(tree, l) {
			stopping = append(stopping, l.done)
			continue
		}
		_ = tree.Kill()
	}
	timer := time.NewTimer(bound)
	defer timer.Stop()
	expired := false
wait:
	for _, done := range stopping {
		select {
		case <-done:
		case <-timer.C:
			expired = true
			break wait
		case <-hurry:
			killAll(trees)
			return
		}
	}
	killAll(trees)
	left := timer.C
	if expired {
		grace := time.NewTimer(relayGrace)
		defer grace.Stop()
		left = grace.C
	}
	select {
	case <-relaysIdle():
	case <-left:
	case <-hurry:
	}
}

func killAll(trees map[*proctree.Tree]liveTree) {
	for tree := range trees {
		_ = tree.Kill()
	}
}

// askAgain asks a tree to stop once more: a suite by the second stop signal,
// a tree with no stop file by an interrupt, which a shared fixture process
// takes again however often it came.
func askAgain(tree *proctree.Tree, l liveTree) bool {
	if l.stopFile == "" {
		return tree.Interrupt() == nil
	}
	pid := readPID(protocol.StopPIDPath(l.stopFile))
	return pid > 0 && stopAgain(pid, l.root) == nil
}
