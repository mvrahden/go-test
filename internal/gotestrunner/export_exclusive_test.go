package gotestrunner

import (
	"context"
	"io"
	"strconv"
)

// ExportScopeKey is a module and a group, "" for the default group.
type ExportScopeKey = scopeKey

var ExportExclusiveLockPath = exclusiveLockPath

var ExportTryLockFile = tryLockFile

// ExportCrossRunLocks is a run's locks under root, as if a parent run held
// inherited, saying on stderr what it waits for.
func ExportCrossRunLocks(root string, inherited []string, stderr io.Writer) *ExportLocks {
	l := &crossRunLocks{root: root, inherited: map[string]bool{}, stderr: stderr}
	for _, p := range inherited {
		l.inherited[p] = true
	}
	return &ExportLocks{l}
}

type ExportLocks struct{ l *crossRunLocks }

// HoldScope plans one scope of group whose suites live in modules, their
// packages sorted in that order, the scope's Exclusive suites among them when exclusive, and takes
// its hold.
func (e *ExportLocks) HoldScope(ctx context.Context, group string, exclusive bool, modules ...string) (release func(), err error) {
	sc := suiteScope{Group: group, Bulk: map[string][]*PlannedSuite{}, Tail: map[string][]*PlannedSuite{}}
	for i, m := range modules {
		phase := sc.Bulk
		if exclusive {
			phase = sc.Tail
		}
		// Packages sort in the order the modules are given.
		pkg := "p" + strconv.Itoa(i)
		phase[pkg] = append(phase[pkg], &PlannedSuite{Package: pkg, Module: m, Group: group, Exclusive: exclusive})
	}
	h := planScopeHolds(e.l, []suiteScope{sc})[0]
	if h == nil {
		return func() {}, nil
	}
	if err := h.take(ctx); err != nil {
		return nil, err
	}
	return h.done, nil
}

func (e *ExportLocks) WithHeld(env []string) []string { return e.l.withHeld(env) }

func (e *ExportLocks) CutWait() string { return e.l.cutWait() }
