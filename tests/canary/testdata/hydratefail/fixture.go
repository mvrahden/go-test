// Package hydratefail has a shared fixture whose Hydrate fails partway.
package hydratefail

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

var markMu sync.Mutex

// mark appends one line to the markers file the canary reads back.
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

// PoolSharedFixture opens a client in Hydrate and fails before it is ready.
type PoolSharedFixture struct {
	Addr   string
	client *struct{}
}

func (f *PoolSharedFixture) BeforeAll(ctx context.Context) error {
	f.Addr = "up"
	return nil
}

func (f *PoolSharedFixture) AfterAll(ctx context.Context) error { return nil }

func (f *PoolSharedFixture) Hydrate(ctx context.Context) error {
	f.client = &struct{}{}
	mark("pool.Hydrate")
	return errors.New("the client never became ready")
}

func (f *PoolSharedFixture) Dehydrate(ctx context.Context) error {
	if f.client != nil {
		mark("pool.Dehydrate")
	}
	return nil
}
