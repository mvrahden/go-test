// Package fixataildep has an exclusive suite whose shared fixture depends on a
// second shared fixture no other suite needs, so both start at the barrier
// after the bulk suites. The dependent's name sorts before its dependency's.
package fixataildep

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

var markMu sync.Mutex

// mark appends one line to the markers file the run under test reads back.
func mark(event string) {
	markMu.Lock()
	defer markMu.Unlock()
	f, err := os.OpenFile(filepath.Join(os.Getenv("GOTEST_CANARY_DIR"), "markers"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(event + "\n")
}

// AppSharedFixture is up once its DB is.
type AppSharedFixture struct {
	DB    *DBSharedFixture
	State string
}

func (f *AppSharedFixture) SharedFixtureConfig() gotest.FixtureConfig {
	return gotest.FixtureConfig{Timeout: 2 * time.Second}
}

func (f *AppSharedFixture) BeforeAll(ctx context.Context) error {
	mark("app.BeforeAll")
	if f.DB.State != "up" {
		return errors.New("DB is not up")
	}
	f.State = "up"
	return nil
}

func (f *AppSharedFixture) AfterAll(ctx context.Context) error {
	mark("app.AfterAll")
	return nil
}

// DBSharedFixture is the dependency only the tail needs.
type DBSharedFixture struct {
	State string
}

func (f *DBSharedFixture) SharedFixtureConfig() gotest.FixtureConfig {
	return gotest.FixtureConfig{Timeout: 2 * time.Second}
}

func (f *DBSharedFixture) BeforeAll(ctx context.Context) error {
	mark("db.BeforeAll")
	f.State = "up"
	return nil
}

func (f *DBSharedFixture) AfterAll(ctx context.Context) error {
	mark("db.AfterAll")
	return nil
}
