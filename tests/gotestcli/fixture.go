// Package gotestcli holds the shared fixture every CLI-driving suite uses,
// so a run builds the gotest binary once instead of once per suite process.
package gotestcli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// BinarySharedFixture is the CLI built from the checkout under test.
type BinarySharedFixture struct {
	Binary   string // the built CLI
	RepoRoot string // the checkout it was built from
}

// SharedFixtureConfig: a cold build competes with the run's own compiles.
func (f *BinarySharedFixture) SharedFixtureConfig() gotest.FixtureConfig {
	cfg := gotest.DefaultFixtureConfig()
	cfg.Timeout = 5 * time.Minute
	return cfg
}

func (f *BinarySharedFixture) BeforeAll(ctx context.Context) error {
	root, err := repoRoot()
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "gotest-cli-*")
	if err != nil {
		return err
	}
	binary, err := testkit.BuildCLI(ctx, root, dir)
	if err != nil {
		_ = os.RemoveAll(dir)
		return err
	}
	f.Binary, f.RepoRoot = binary, root
	return nil
}

func (f *BinarySharedFixture) AfterAll(ctx context.Context) error {
	if f.Binary == "" {
		return nil
	}
	return os.RemoveAll(filepath.Dir(f.Binary))
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
