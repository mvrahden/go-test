package gotestrunner

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/mvrahden/go-test/internal/gotestgen"
)

// fixtureLedger counts, for each shared fixture, the planned suites that
// still need it, its dependents' needs included: a fixture whose count
// reaches zero is needed by no suite left to run, and neither is any fixture
// depending on it.
type fixtureLedger struct {
	mu     sync.Mutex
	needs  map[string]map[string][]string // package → suite → closed keys
	counts map[string]int
	done   map[[2]string]bool
}

// newFixtureLedger counts the needs of the planned suites, package → test
// function names, as planSuitePhases returns them.
func newFixtureLedger(planned []map[string][]string, reqKeys map[string]map[string][]string, fixtures []gotestgen.SharedFixtureInfo) *fixtureLedger {
	l := &fixtureLedger{needs: map[string]map[string][]string{}, counts: map[string]int{}, done: map[[2]string]bool{}}
	for _, phase := range planned {
		for pkg, suites := range phase {
			for _, suite := range suites {
				keys := sortedKeys(aliveFixtureKeys(map[string][]string{pkg: {suite}}, reqKeys, fixtures))
				if l.needs[pkg] == nil {
					l.needs[pkg] = map[string][]string{}
				}
				l.needs[pkg][suite] = keys
				for _, key := range keys {
					l.counts[key]++
				}
			}
		}
	}
	return l
}

// Done records that the suite will not run, or has run and its process is
// gone, and returns the keys no remaining planned suite needs, sorted. A
// suite counts once, however often it is reported.
func (l *fixtureLedger) Done(pkg, suite string) []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	id := [2]string{pkg, suite}
	keys, planned := l.needs[pkg][suite]
	if !planned || l.done[id] {
		return nil
	}
	l.done[id] = true
	var free []string
	for _, key := range keys {
		l.counts[key]--
		if l.counts[key] == 0 {
			free = append(free, key)
		}
	}
	return free
}

// DoneExcept is Done for every planned suite of pkg not in kept: the suites
// a build failure or the selection left without a process.
func (l *fixtureLedger) DoneExcept(pkg string, kept map[string]bool) []string {
	l.mu.Lock()
	suites := make([]string, 0, len(l.needs[pkg]))
	for suite := range l.needs[pkg] {
		if !kept[suite] {
			suites = append(suites, suite)
		}
	}
	l.mu.Unlock()
	sort.Strings(suites)
	var free []string
	for _, suite := range suites {
		free = append(free, l.Done(pkg, suite)...)
	}
	sort.Strings(free)
	return free
}

func sortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// fixtureReleaser releases the keys the ledger frees on one goroutine of its
// own, one teardown command at a time, so a suite that frees a fixture never
// waits for its release, and neither does a start: the process serves
// commands concurrently. Keys freed while a command is in flight go out
// together in the next.
type fixtureReleaser struct {
	mu      sync.Mutex
	pending []string
	busy    bool
	idle    *sync.Cond
	wake    chan struct{}
	errs    []error
	stopped chan struct{}
}

// startFixtureReleaser releases through the process ready returns once its
// up-front phase is over; nil there leaves every key to the terminal
// teardown. It stops when ctx ends.
func startFixtureReleaser(ctx context.Context, ready func() *SharedFixtureProcess) *fixtureReleaser {
	r := &fixtureReleaser{wake: make(chan struct{}, 1), stopped: make(chan struct{})}
	r.idle = sync.NewCond(&r.mu)
	go func() {
		defer close(r.stopped)
		defer func() {
			r.mu.Lock()
			r.pending, r.busy = nil, false
			r.idle.Broadcast()
			r.mu.Unlock()
		}()
		var proc *SharedFixtureProcess
		resolved := false
		for {
			select {
			case <-r.wake:
			case <-ctx.Done():
				return
			}
			r.mu.Lock()
			batch := r.pending
			r.pending = nil
			r.busy = len(batch) > 0
			r.mu.Unlock()
			if len(batch) == 0 {
				continue
			}
			if !resolved {
				proc, resolved = ready(), true
			}
			if proc != nil && ctx.Err() == nil {
				if err := proc.TeardownKeys(ctx, batch, proc.teardownBudget()); err != nil && ctx.Err() == nil {
					r.mu.Lock()
					r.errs = append(r.errs, fmt.Errorf("early shared fixture teardown: %w", err))
					r.mu.Unlock()
				}
			}
			r.mu.Lock()
			r.busy = false
			if len(r.pending) > 0 {
				r.signal()
			}
			r.idle.Broadcast()
			r.mu.Unlock()
		}
	}()
	return r
}

// Release queues keys for release.
func (r *fixtureReleaser) Release(keys []string) {
	if len(keys) == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pending = append(r.pending, keys...)
	r.signal()
}

func (r *fixtureReleaser) signal() {
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

// Drain waits until no key is queued or in flight.
func (r *fixtureReleaser) Drain() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for len(r.pending) > 0 || r.busy {
		select {
		case <-r.stopped:
			return
		default:
		}
		r.idle.Wait()
	}
}

// Err is every release that failed, once the releaser has stopped.
func (r *fixtureReleaser) Err() error {
	<-r.stopped
	r.mu.Lock()
	defer r.mu.Unlock()
	return errors.Join(r.errs...)
}
