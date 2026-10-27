// Package fixadag releases shared fixtures together: the bulk suite's fixture
// once the bulk is done, then the tail's three at the end of the run, Child depending
// on A and B independent of both. With FIXADAG_HANG set, Child's AfterAll
// never returns.
package fixadag

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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

// marked waits up to 20s for event in the markers file.
func marked(event string) bool {
	path := filepath.Join(os.Getenv("GOTEST_CANARY_DIR"), "markers")
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		data, _ := os.ReadFile(path)
		for _, line := range strings.Split(string(data), "\n") {
			if line == event {
				return true
			}
		}
	}
	return false
}

// release marks the AfterAll of name around half a second of work.
func release(name string) error {
	mark(name + ".AfterAll begins")
	time.Sleep(500 * time.Millisecond)
	mark(name + ".AfterAll ends")
	return nil
}

// BulkSharedFixture is the bulk suite's alone, released once the bulk is done.
type BulkSharedFixture struct{}

func (f *BulkSharedFixture) SharedFixtureConfig() gotest.FixtureConfig {
	return gotest.FixtureConfig{Timeout: 10 * time.Second}
}
func (f *BulkSharedFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *BulkSharedFixture) AfterAll(ctx context.Context) error  { return release("bulk") }

type ASharedFixture struct{}

func (f *ASharedFixture) SharedFixtureConfig() gotest.FixtureConfig {
	return gotest.FixtureConfig{Timeout: 2 * time.Second}
}
func (f *ASharedFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *ASharedFixture) AfterAll(ctx context.Context) error  { return release("a") }

type BSharedFixture struct{}

func (f *BSharedFixture) SharedFixtureConfig() gotest.FixtureConfig {
	return gotest.FixtureConfig{Timeout: 2 * time.Second}
}
func (f *BSharedFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *BSharedFixture) AfterAll(ctx context.Context) error  { return release("b") }

// ChildSharedFixture depends on A.
type ChildSharedFixture struct {
	A *ASharedFixture
}

func (f *ChildSharedFixture) SharedFixtureConfig() gotest.FixtureConfig {
	return gotest.FixtureConfig{Timeout: 3 * time.Second}
}
func (f *ChildSharedFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *ChildSharedFixture) AfterAll(ctx context.Context) error {
	if os.Getenv("FIXADAG_HANG") != "" {
		mark("child.AfterAll begins")
		select {}
	}
	return release("child")
}
