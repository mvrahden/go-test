package fixtureteardown

import (
	"context"
	"os"
	"path/filepath"
)

// AlivePath is the file the fixture holds while it is set up.
func AlivePath() string { return filepath.Join(os.Getenv("GOTEST_CANARY_DIR"), "fixture-alive") }

// LedgerFixture writes a marker while it is up and another once it was torn
// down; the canary reads the second one back after the run.
type LedgerFixture struct{}

func (f *LedgerFixture) BeforeAll(ctx context.Context) error {
	return os.WriteFile(AlivePath(), []byte("alive"), 0o600)
}

func (f *LedgerFixture) AfterAll(ctx context.Context) error {
	if err := os.Remove(AlivePath()); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(os.Getenv("GOTEST_CANARY_DIR"), "fixture-afterall"), []byte("done"), 0o600)
}
