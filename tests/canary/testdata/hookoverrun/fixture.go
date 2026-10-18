// Package hookoverrun is a fixture-bound suite whose fixture's BeforeEach
// sleeps past the suite's Timeout; the canary reads what that does to the
// method.
package hookoverrun

import (
	"context"
	"os"
	"path/filepath"
	"time"
)

// mark appends one line to the markers file the canary reads back.
func mark(event string) {
	f, err := os.OpenFile(filepath.Join(os.Getenv("GOTEST_CANARY_DIR"), "markers"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(event + "\n")
}

// SlowFixture takes longer to prepare a method than the suite allows one.
type SlowFixture struct{}

func (f *SlowFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *SlowFixture) AfterAll(ctx context.Context) error  { return nil }

func (f *SlowFixture) BeforeEach(ctx context.Context) error {
	time.Sleep(1500 * time.Millisecond)
	return nil
}
