// Package gotestcli holds the shared fixture every CLI-driving suite uses,
// so a run builds the gotest binary once instead of once per suite process.
package gotestcli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"

	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// BinarySharedFixture is the CLI built from the checkout under test, and the
// gotest cache every suite binding it runs that CLI with.
type BinarySharedFixture struct {
	Binary   string // the built CLI
	RepoRoot string // the checkout it was built from
	CacheDir string // the run's gotest cache, removed with the run
	// TmpDir is the temp dir for the CLIs the suites start, removed with
	// the run: their sweeps of leftover work dirs stay off the real one.
	TmpDir string
}

// SharedFixtureConfig: no deadline: this fixture compiles the CLI, so its time
// is not a verdict; the run's --setup-timeout still bounds a hang.
func (f *BinarySharedFixture) SharedFixtureConfig() gotest.FixtureConfig {
	cfg := gotest.DefaultFixtureConfig()
	cfg.Timeout = gotest.NoDeadline
	return cfg
}

func (f *BinarySharedFixture) BeforeAll(ctx context.Context) error {
	root, err := repoRoot()
	if err != nil {
		return err
	}
	// Stamped, so a run that kills this process leaves a dir the next sweep takes.
	dir, err := testkit.NewCLIDir()
	if err != nil {
		return err
	}
	binary, err := testkit.BuildCLI(ctx, root, dir)
	if err != nil {
		_ = os.RemoveAll(dir)
		return err
	}
	tmp := filepath.Join(dir, "tmp")
	if err := os.Mkdir(tmp, 0o700); err != nil {
		_ = os.RemoveAll(dir)
		return err
	}
	f.Binary, f.RepoRoot, f.CacheDir, f.TmpDir = binary, root, filepath.Join(dir, "cache"), tmp
	return nil
}

func (f *BinarySharedFixture) AfterAll(ctx context.Context) error {
	if f.Binary == "" {
		return nil
	}
	return os.RemoveAll(filepath.Dir(f.Binary))
}

// Hydrate gives every CLI the suite starts the run's cache, not the user's:
// the suites run it from temp dirs, whose entries no later run hits.
func (f *BinarySharedFixture) Hydrate(ctx context.Context) error {
	return testkit.PrivateCache(f.CacheDir)
}

// repoRoot is the checkout this file was compiled from, so a copied tree
// (the mutation drill) builds its own CLI.
func repoRoot() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok || !filepath.IsAbs(file) {
		return "", errors.New("gotestcli: cannot locate the checkout (source paths trimmed?)")
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(file))), nil
}

// Dehydrate drops the cache Hydrate set; the run removes the directory.
func (f *BinarySharedFixture) Dehydrate(ctx context.Context) error {
	return os.Unsetenv(protocol.EnvCacheDir)
}
