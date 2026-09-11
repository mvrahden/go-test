// Package fixtures holds a shared fixture that records its own lifecycle as
// marker files, so a test outside the run can see whether teardown ran.
package fixtures

import (
	"context"
	"os"
	"path/filepath"
	"time"
)

// ProbeSharedFixture writes "fixture-up" when it starts and "fixture-down"
// when it is torn down, into the directory GOTEST_SHUTDOWN_DIR names.
type ProbeSharedFixture struct {
	Dir string
}

func (f *ProbeSharedFixture) BeforeAll(ctx context.Context) error {
	f.Dir = os.Getenv("GOTEST_SHUTDOWN_DIR")
	return os.WriteFile(filepath.Join(f.Dir, "fixture-up"), []byte("up"), 0o600)
}

// AfterAll writes "fixture-tearing-down" first and, when
// GOTEST_SHUTDOWN_TEARDOWN_SLEEP names a duration, holds the teardown open
// that long so a test can interrupt the run while it is tearing down.
func (f *ProbeSharedFixture) AfterAll(ctx context.Context) error {
	if err := os.WriteFile(filepath.Join(f.Dir, "fixture-tearing-down"), []byte("tearing down"), 0o600); err != nil {
		return err
	}
	if d, err := time.ParseDuration(os.Getenv("GOTEST_SHUTDOWN_TEARDOWN_SLEEP")); err == nil {
		time.Sleep(d)
	}
	return os.WriteFile(filepath.Join(f.Dir, "fixture-down"), []byte("down"), 0o600)
}
