package fixturefuzzing

import (
	"context"
	"os"
	"path/filepath"
)

// AlivePath is the file the fixture holds while it is set up.
func AlivePath() string { return filepath.Join(os.Getenv("GOTEST_CANARY_DIR"), "fixture-alive") }

// LedgerFixture holds a marker file between BeforeAll and AfterAll.
type LedgerFixture struct{}

func (f *LedgerFixture) BeforeAll(ctx context.Context) error {
	return os.WriteFile(AlivePath(), []byte("alive"), 0o600)
}

func (f *LedgerFixture) AfterAll(ctx context.Context) error { return os.Remove(AlivePath()) }
