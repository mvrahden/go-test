package testkit

import "time"

// ExportIsRepoModule exposes the go.mod check RepoRoot walks with.
var ExportIsRepoModule = isRepoModule

// ExportCacheDir exposes where a run's gotest cache goes.
var ExportCacheDir = ChildCacheDir

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
