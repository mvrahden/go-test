// Package b holds an unfocused suite bound to a package and a shared fixture.
package b

import (
	"context"
	"os"
	"path/filepath"
)

func mark(line string) {
	w, err := os.OpenFile(filepath.Join(os.Getenv("GOTEST_CANARY_DIR"), "markers"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err == nil {
		_, _ = w.WriteString(line + "\n")
		_ = w.Close()
	}
}

// MarkSharedFixture records that it came up.
type MarkSharedFixture struct {
	State string
}

func (f *MarkSharedFixture) BeforeAll(ctx context.Context) error {
	mark("shared up")
	f.State = "up"
	return nil
}

func (f *MarkSharedFixture) AfterAll(ctx context.Context) error { return nil }
