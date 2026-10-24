package gotestrunner

import (
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

// Abandon gives up on every process tree still running, for a CLI about to
// exit without waiting for them. Each suite process that published its pid
// is sent a second stop signal, which releases its fixtures at once, and a
// tree with no stop file, the shared fixture's, is asked to tear down; a tree
// neither reaches is killed at once. Within bound of that, or as soon as
// hurry closes, every tree still running is killed, so no process outlives
// the CLI.
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
wait:
	for _, done := range stopping {
		select {
		case <-done:
		case <-timer.C:
			break wait
		case <-hurry:
			break wait
		}
	}
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
