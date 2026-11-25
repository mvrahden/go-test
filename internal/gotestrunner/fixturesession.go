package gotestrunner

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/internal/schedinfo"
)

// fixtureSession is a run's shared fixtures: the process behind them, the
// up-front phase and every later start. Dispatch names fixtures by key and
// never touches the process.
type fixtureSession struct {
	proc *SharedFixtureProcess
	// started closes once the process started or failed to; over once the
	// up-front phase ended, however it ended.
	started, over chan struct{}
	// gaveUp closes when setup failed or timed out: it ends the wait for the
	// fixtures that did not come up, and nothing else. Those that did stay
	// up, the suites that read only them run, and so does every suite that
	// reads none.
	gaveUp   chan struct{}
	startErr error
	// setupErr and timedOut are written by the setup goroutine and read
	// once wg has drained.
	setupErr error
	timedOut bool
	failed   atomic.Bool
	wg       sync.WaitGroup
	up       map[string]bool // every key asked for, up front or later
	lateErr  error
	timeout  time.Duration
}

// startFixtureSession starts the up-front fixtures of win beside the
// compile. ctx bounds the process; streamCtx ends the wait for setup.
// cancelStream is called when the process cannot start because ctx ended.
// before, when set, runs ahead of the start; its error is the stream's end.
func startFixtureSession(ctx, streamCtx context.Context, cancelStream context.CancelFunc, workDir string, win fixtureWindows, setupTimeout time.Duration, buildFlags []string, before func(context.Context) error) *fixtureSession { //nolint:gocritic // hugeParam: once per run
	f := &fixtureSession{
		started: make(chan struct{}),
		over:    make(chan struct{}),
		gaveUp:  make(chan struct{}),
		up:      maps.Clone(win.Bulk),
		timeout: setupTimeout,
	}
	if len(win.Fixtures) == 0 {
		close(f.started)
		close(f.over)
		return f
	}
	f.wg.Add(1)
	go func() {
		defer f.wg.Done()
		defer close(f.over)
		var err error
		// The subprocess is bound to the pipeline ctx, not streamCtx: close
		// is the one owner of shutdown, and it runs only after every suite
		// has stopped. The pipeline ctx stays attached as the safety net so
		// an abnormal runner death still releases the process group.
		if before != nil {
			err = before(streamCtx)
		}
		if err == nil {
			traceDispatch(dispatchEvent{Kind: "up-front", Keys: sortedKeys(win.Bulk)})
			f.proc, err = StartSharedFixtures(ctx, workDir, win.Fixtures, setupTimeout, buildFlags...)
		}
		if err != nil {
			f.startErr = err
			if ctx.Err() == nil && streamCtx.Err() == nil {
				f.fail(fmt.Errorf("shared fixture setup failed: %w", err))
			} else {
				f.failed.Store(true)
				cancelStream()
			}
		}
		close(f.started)
		if err != nil {
			return
		}
		var timedOut *SetupTimeoutError
		switch err := f.proc.WaitSetup(streamCtx, setupTimeout); {
		case err == nil, streamCtx.Err() != nil:
		case errors.As(err, &timedOut):
			f.timedOut = true
			f.fail(fmt.Errorf("%w %s", err, schedinfo.Summary()))
		default:
			f.fail(err)
		}
	}()
	return f
}

// settledFixtureSession is a session whose up-front phase is over: proc
// with what came up, or nil with the error that kept it from starting.
func settledFixtureSession(proc *SharedFixtureProcess, setupErr error, timedOut bool) *fixtureSession {
	f := &fixtureSession{
		proc: proc, setupErr: setupErr, timedOut: timedOut,
		started: make(chan struct{}), over: make(chan struct{}), gaveUp: make(chan struct{}), up: map[string]bool{},
	}
	close(f.started)
	close(f.over)
	if setupErr != nil {
		f.failed.Store(true)
		close(f.gaveUp)
	}
	return f
}

func (f *fixtureSession) fail(err error) {
	f.setupErr = err
	f.failed.Store(true)
	close(f.gaveUp)
}

// await waits until each of keys came up or setup gave up on it, and
// returns the first that is not up, "" when all are. ok is false when ctx
// ended first.
func (f *fixtureSession) await(ctx context.Context, keys []string) (missing string, ok bool) {
	if len(keys) == 0 {
		return "", true
	}
	select {
	case <-f.started:
	case <-ctx.Done():
		return "", false
	}
	if f.startErr != nil || f.proc == nil {
		return keys[0], true
	}
	for _, key := range keys {
		// A fixture the process does not know has no channel, and a nil
		// channel never delivers.
		select {
		case <-f.proc.Ready(key):
		case <-f.proc.AllDone():
		case <-f.gaveUp:
		case <-ctx.Done():
			return "", false
		}
		// A select picks at random among ready cases; by the time setup is
		// over, every fixture that came up has had its channel closed.
		if !f.proc.Came(key) {
			return key, true
		}
	}
	return "", true
}

// missing returns the first of keys that is not up, "" when all are.
func (f *fixtureSession) missing(keys []string) string {
	return missingFixture(f.proc, keys)
}

// settle waits until the up-front phase is over.
func (f *fixtureSession) settle() { f.wg.Wait() }

// startFor starts the fixtures suites read that are not up yet, once the
// up-front phase is over. what names them in a failure. A failed setup
// leaves the process serving commands, so these still start; one that timed
// out is still busy with the up-front phase and would answer no command.
func (f *fixtureSession) startFor(ctx context.Context, suites []*PlannedSuite, fixtures []gotestgen.SharedFixtureInfo, what string) {
	if f.proc == nil || f.timedOut || ctx.Err() != nil {
		return
	}
	need := aliveFixtureKeys(suites, fixtures)
	start := diffKeys(need, f.up)
	if len(start) == 0 {
		return
	}
	for _, key := range start {
		f.up[key] = true
	}
	traceDispatch(dispatchEvent{Kind: "start", Keys: start, For: what})
	if err := f.proc.StartKeys(ctx, start, f.timeout); err != nil {
		f.lateErr = errors.Join(f.lateErr, fmt.Errorf("shared fixture start for %s: %w", what, err))
	}
}

// env is base plus the state file of the keys pkg's suite reads; base
// itself when it reads none.
func (f *fixtureSession) env(base []string, pkg, suite string, keys []string) ([]string, error) {
	if len(keys) == 0 {
		return base, nil
	}
	stateFile, err := f.proc.WriteSuiteStateFile(pkg, suite, keys)
	if err != nil {
		return nil, err
	}
	env := make([]string, len(base), len(base)+1)
	copy(env, base)
	return append(env, protocol.EnvSharedStateFile+"="+stateFile), nil
}

// releasable is the process releases go to, nil when setup timed out or
// nothing started; it blocks until the up-front phase is over or ctx ends.
// One still busy with the up-front phase after a timeout would answer no
// command, and the terminal teardown owns its fixtures.
func (f *fixtureSession) releasable(ctx context.Context) *SharedFixtureProcess {
	select {
	case <-f.over:
	case <-ctx.Done():
		return nil
	}
	if f.proc == nil || f.timedOut {
		return nil
	}
	return f.proc
}

// failedSetup reports whether the up-front phase failed.
func (f *fixtureSession) failedSetup() bool { return f.failed.Load() }

// close tears the process down and returns the setup and teardown errors
// with the fixtures' reports attached. releaseErr is what the releaser
// collected.
func (f *fixtureSession) close(releaseErr error) (setupErr, teardownErr error) {
	setupErr = f.setupErr
	if f.proc != nil {
		traceDispatch(dispatchEvent{Kind: "teardown"})
		teardownErr = errors.Join(f.lateErr, releaseErr, f.proc.Teardown())
		reports := f.proc.TakeFailures()
		setupErr = withReports(setupErr, reports, false)
		teardownErr = teardownVerdict(teardownErr, reports)
	}
	return setupErr, teardownErr
}
