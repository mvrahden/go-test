package testkit

import (
	"context"
	"os"
	"path/filepath"
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

// ExportCLIName is the CLI binary's file name.
var ExportCLIName = cliName

// ExportNewStampedDir creates a CLI dir in parent stamped with pid, without
// sweeping.
var ExportNewStampedDir = newStampedDir

// The names a store gives a published build, a use record and a build
// marker, each followed by the build's key.
const (
	ExportBuildPrefix    = buildPrefix
	ExportUsePrefix      = usePrefix
	ExportBuildingPrefix = buildingPrefix
)

// ExportStore is a CLI store used as one process.
type ExportStore struct{ s store }

// NewExportStore is the store in dir used as pid, judging pids by alive,
// keeping a published build for linger after its last use.
func NewExportStore(dir string, pid int, alive func(int) bool, linger time.Duration) ExportStore {
	return ExportStore{store{dir: dir, pid: pid, rules: sweepRules{alive: alive, linger: linger, grace: cliDirGrace, wait: buildWait}}}
}

// Get copies key's build into private, building it with build when none is
// published.
func (e ExportStore) Get(ctx context.Context, key, private string, build func(ctx context.Context, dir string) (string, error)) (string, error) {
	return e.s.get(ctx, key, private, build)
}

// Sweep sweeps the store.
func (e ExportStore) Sweep() { e.s.sweep() }

// ExportFakeBuilds makes every reuse key and build with key and build until
// restore.
func ExportFakeBuilds(key func(ctx context.Context, root string) (string, error), build func(ctx context.Context, root, dir string) (string, error)) (restore func()) {
	keyOf, buildOf = key, build
	return func() { keyOf, buildOf = cliKey, buildCLI }
}

// ExportBeforeMark runs hook before each try to create a build marker until
// restore.
func ExportBeforeMark(hook func(marker string)) (restore func()) {
	beforeMark = hook
	return func() { beforeMark = func(string) {} }
}

// ExportBeforeTake runs hook just before a marker judged dead is removed,
// until restore.
func ExportBeforeTake(hook func(marker string)) (restore func()) {
	beforeTake = hook
	return func() { beforeTake = func(string) {} }
}

// ExportPlantMarker puts key's build marker in dir, held by pid, and returns
// its stamp.
func ExportPlantMarker(dir, key, pid string) (string, error) {
	marker := filepath.Join(dir, buildingPrefix+key)
	if err := os.MkdirAll(marker, 0o750); err != nil {
		return "", err
	}
	stamp := filepath.Join(marker, pid+"-planted")
	return stamp, os.WriteFile(stamp, nil, 0o600)
}

// ExportRenameMarker moves staged build markers into place with rename until
// restore.
func ExportRenameMarker(rename func(from, to string) error) (restore func()) {
	renameMarker = rename
	return func() { renameMarker = os.Rename }
}
