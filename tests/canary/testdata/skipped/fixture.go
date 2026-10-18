package skipped

import (
	"context"
	"os"
	"path/filepath"
)

// MarkSharedFixture records that it came up.
type MarkSharedFixture struct {
	State string
}

func (f *MarkSharedFixture) BeforeAll(ctx context.Context) error {
	w, err := os.OpenFile(filepath.Join(os.Getenv("GOTEST_CANARY_DIR"), "markers"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err == nil {
		_, _ = w.WriteString("shared up\n")
		_ = w.Close()
	}
	f.State = "up"
	return nil
}

func (f *MarkSharedFixture) AfterAll(ctx context.Context) error { return nil }
