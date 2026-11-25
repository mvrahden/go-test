package gotestrunner_test

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// ExclusiveLockTestSuite: a run holds its exclusive scopes across runs by a
// lock file per module and group under the cache, waits for one another run
// holds until its context ends, and takes a scope's modules in order.
type ExclusiveLockTestSuite struct{}

// SuiteConfig runs the methods in parallel: each locks under a cache root
// of its own.
func (s *ExclusiveLockTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

// lockedText is a writer two goroutines share.
type lockedText struct {
	mu sync.Mutex
	b  strings.Builder
}

func (l *lockedText) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedText) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// saysWaiting returns once stderr carries a wait line.
func saysWaiting(t *gotest.T, stderr *lockedText) {
	gotest.Eventually(t, 10*time.Second, 5*time.Millisecond, func(poll *gotest.R) {
		gotest.Contains(poll, stderr.String(), "waits for another gotest run")
	})
}

func (s *ExclusiveLockTestSuite) TestTheKeyIsTheModuleAndTheGroup(t *gotest.T) {
	root := t.TempDir()
	path := func(module, group string) string {
		return gotestrunner.ExportExclusiveLockPath(root, gotestrunner.ExportScopeKey{Module: module, Group: group})
	}
	t.When("two scopes' lock files are compared", func(w *gotest.T) {
		w.It("gives one module and group one file under the cache's exclusive directory", func(it *gotest.T) {
			gotest.Equal(it, path("example.com/m", ""), path("example.com/m", ""))
			gotest.Equal(it, filepath.Join(root, "exclusive"), filepath.Dir(path("example.com/m", "")))
		})
		w.It("gives another module or another group another file", func(it *gotest.T) {
			gotest.NotEqual(it, path("example.com/m", ""), path("example.com/n", ""))
			gotest.NotEqual(it, path("example.com/m", ""), path("example.com/m", "e2e"))
		})
	})
}

func (s *ExclusiveLockTestSuite) TestAWaitSaysWhoHoldsIt(t *gotest.T) {
	root := t.TempDir()
	holder := gotestrunner.ExportCrossRunLocks(root, nil, new(lockedText))
	release, err := holder.HoldScope(t.Context(), "e2e", false, "example.com/m")
	gotest.NoError(t, err)
	stderr := new(lockedText)
	got := make(chan error, 1)
	gotest.Go(t, func() {
		done, err := gotestrunner.ExportCrossRunLocks(root, nil, stderr).HoldScope(t.Context(), "e2e", false, "example.com/m")
		if err == nil {
			done()
		}
		got <- err
	})
	saysWaiting(t, stderr)
	waiting := stderr.String()
	release()
	t.When("a run's group is held by another run", func(w *gotest.T) {
		w.It("says once which scope waits and the holder's pid", func(it *gotest.T) {
			gotest.Equal(it, 1, strings.Count(waiting, "\n"), "stderr: %s", waiting)
			gotest.Contains(it, waiting, "gotest: group e2e of example.com/m waits for another gotest run (pid "+strconv.Itoa(os.Getpid())+" in ")
		})
		w.It("takes the scope once the other run released it", func(it *gotest.T) {
			gotest.NoError(it, <-got)
		})
	})
}

func (s *ExclusiveLockTestSuite) TestAWaitEndsWithItsContext(t *gotest.T) {
	root := t.TempDir()
	release, err := gotestrunner.ExportCrossRunLocks(root, nil, new(lockedText)).HoldScope(t.Context(), "", true, "example.com/m")
	gotest.NoError(t, err)
	stderr := new(lockedText)
	waiter := gotestrunner.ExportCrossRunLocks(root, nil, stderr)
	ctx, cancel := context.WithCancel(t.Context())
	got := make(chan error, 1)
	gotest.Go(t, func() {
		_, err := waiter.HoldScope(ctx, "", true, "example.com/m")
		got <- err
	})
	saysWaiting(t, stderr)
	cancel()
	waitErr := <-got
	release()
	t.When("a run's context ends while it waits for another run", func(w *gotest.T) {
		w.It("ends the wait with the context's error, naming the scope it waited for", func(it *gotest.T) {
			gotest.ErrorIs(it, waitErr, context.Canceled)
			gotest.Equal(it, "the exclusive suites of example.com/m", waiter.CutWait())
		})
		w.It("leaves the lock free once the holder released it", func(it *gotest.T) {
			done, err := gotestrunner.ExportCrossRunLocks(root, nil, new(lockedText)).HoldScope(t.Context(), "", true, "example.com/m")
			gotest.NoError(it, err)
			done()
		})
	})
}

func (s *ExclusiveLockTestSuite) TestAScopeTakesItsModulesInOrder(t *gotest.T) {
	root := t.TempDir()
	lockOf := func(module string) string {
		return gotestrunner.ExportExclusiveLockPath(root, gotestrunner.ExportScopeKey{Module: module})
	}
	gotest.NoError(t, os.MkdirAll(filepath.Join(root, "exclusive"), 0o755))
	// Another run holds the later module.
	unlock, err := gotestrunner.ExportTryLockFile(lockOf("b.example/m"))
	gotest.NoError(t, err)
	stderr := new(lockedText)
	got := make(chan error, 1)
	gotest.Go(t, func() {
		done, err := gotestrunner.ExportCrossRunLocks(root, nil, stderr).HoldScope(t.Context(), "", true, "b.example/m", "a.example/m")
		if err == nil {
			done()
		}
		got <- err
	})
	saysWaiting(t, stderr)
	_, firstFree := gotestrunner.ExportTryLockFile(lockOf("a.example/m"))
	unlock()
	t.When("a scope of two modules waits for the later one", func(w *gotest.T) {
		w.It("holds the earlier one already, whatever order its suites came in", func(it *gotest.T) {
			gotest.Error(it, firstFree)
			gotest.Contains(it, stderr.String(), "of b.example/m waits")
			gotest.NoError(it, <-got)
		})
	})
}

func (s *ExclusiveLockTestSuite) TestAnInheritedLockIsTheRunsOwn(t *gotest.T) {
	root := t.TempDir()
	parent := gotestrunner.ExportCrossRunLocks(root, nil, new(lockedText))
	release, err := parent.HoldScope(t.Context(), "", true, "example.com/m")
	gotest.NoError(t, err)
	env := parent.WithHeld(nil)
	path := gotestrunner.ExportExclusiveLockPath(root, gotestrunner.ExportScopeKey{Module: "example.com/m"})
	stderr := new(lockedText)
	done, childErr := gotestrunner.ExportCrossRunLocks(root, []string{path}, stderr).HoldScope(t.Context(), "", true, "example.com/m")
	release()
	t.When("a run a holding run's suite started needs the scope its parent holds", func(w *gotest.T) {
		w.It("hands the suite's process the lock files it holds", func(it *gotest.T) {
			gotest.Equal(it, []string{"GOTEST_EXCLUSIVE_HELD=" + path}, env)
		})
		w.It("takes the scope without waiting", func(it *gotest.T) {
			gotest.NoError(it, childErr)
			gotest.Empty(it, stderr.String())
			done()
		})
	})
}
