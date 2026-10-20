package testkit

import (
	"context"
	"errors"
	"os"
	"sync"
	"time"
)

// ExportIsRepoModule exposes the go.mod check RepoRoot walks with.
var ExportIsRepoModule = isRepoModule

// ExportResolveCLI exposes the build-or-reuse step cli runs once per process.
var ExportResolveCLI = resolveCLI

// ExportCLIKey exposes what a built CLI is keyed by.
var ExportCLIKey = cliKey

// ExportCLIDirAlive exposes whether a CLI dir belongs to a live process.
var ExportCLIDirAlive = cliDirAlive

// ExportClaimAs claims a published build for pid, judging stamps by alive.
var ExportClaimAs = claimAs

// ExportSweepDir sweeps one CLI dir, judging stamps by alive, with linger for
// a published build.
func ExportSweepDir(dir string, alive func(int) bool, linger time.Duration) {
	sweepCLIDir(dir, sweepRules{alive: alive, linger: linger, grace: cliDirGrace})
}

// ExportKeyedPrefix starts a published build's dir name.
const ExportKeyedPrefix = keyedPrefix

// ExportCLIName is the CLI binary's file name.
var ExportCLIName = cliName

// ExportNewStampedDir creates a CLI dir stamped with pid, without sweeping.
var ExportNewStampedDir = newStampedDir

// ExportPublish moves a build to its keyed path the way a builder does.
var ExportPublish = publish

// ExportHandOffLocks makes dir locks behave as LockFileEx does until restore:
// a release hands the lock to a waiter, so a try fails while one waits.
func ExportHandOffLocks() (restore func()) {
	var mu sync.Mutex
	waiting := map[string]int{}
	lockDirFD = func(f *os.File, try bool) error {
		mu.Lock()
		queued := waiting[f.Name()]
		if !try {
			waiting[f.Name()]++
		}
		mu.Unlock()
		if try {
			if queued > 0 {
				return errors.New("lock held for a waiter")
			}
			return lockFD(f, true)
		}
		err := lockFD(f, false)
		mu.Lock()
		waiting[f.Name()]--
		mu.Unlock()
		return err
	}
	return func() { lockDirFD = lockFD }
}

// ExportFcntlLocks makes the build cache's lock behave as fcntl does until
// restore: it never excludes the same process. build replaces the go build.
func ExportFcntlLocks(build func(ctx context.Context, root, dir string) (string, error)) (restore func()) {
	lockCacheFD = func(*os.File, bool) error { return nil }
	buildCachedCLI = build
	return func() { lockCacheFD, buildCachedCLI = lockFD, buildCLI }
}
