// Package fixturepairs holds suites whose package fixtures' per-method hooks
// run in dependency order and go wrong one at a time; every hook marks
// itself.
package fixturepairs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
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

// SlowFixture's BeforeEach sleeps past the Timeout of the suite binding it.
type SlowFixture struct{}

func (f *SlowFixture) BeforeAll(ctx context.Context) error { return nil }

func (f *SlowFixture) BeforeEach(ctx context.Context) error {
	mark("slow.BeforeEach")
	time.Sleep(time.Second)
	return nil
}

func (f *SlowFixture) AfterEach(ctx context.Context) error {
	mark("slow.AfterEach")
	return nil
}

// LaterFixture comes after SlowFixture; its BeforeEach fails once the
// method's context has ended.
type LaterFixture struct {
	Slow *SlowFixture
}

func (f *LaterFixture) BeforeAll(ctx context.Context) error { return nil }

func (f *LaterFixture) BeforeEach(ctx context.Context) error {
	mark("later.BeforeEach")
	return ctx.Err()
}

func (f *LaterFixture) AfterEach(ctx context.Context) error {
	mark("later.AfterEach")
	return nil
}

// AheadFixture has only an AfterEach and comes before BrokenFixture.
type AheadFixture struct{}

func (f *AheadFixture) BeforeAll(ctx context.Context) error { return nil }

func (f *AheadFixture) AfterEach(ctx context.Context) error {
	mark("ahead.AfterEach")
	return nil
}

// BrokenFixture's BeforeEach fails, after AheadFixture's turn.
type BrokenFixture struct {
	Ahead *AheadFixture
}

func (f *BrokenFixture) BeforeAll(ctx context.Context) error { return nil }

func (f *BrokenFixture) BeforeEach(ctx context.Context) error {
	return errors.New("the broken fixture is not ready")
}

// FailingFixture's BeforeEach fails, ahead of BehindFixture's turn.
type FailingFixture struct{}

func (f *FailingFixture) BeforeAll(ctx context.Context) error { return nil }

func (f *FailingFixture) BeforeEach(ctx context.Context) error {
	return errors.New("the failing fixture is not ready")
}

// BehindFixture has only an AfterEach and comes after FailingFixture.
type BehindFixture struct {
	Failing *FailingFixture
}

func (f *BehindFixture) BeforeAll(ctx context.Context) error { return nil }

func (f *BehindFixture) AfterEach(ctx context.Context) error {
	mark("behind.AfterEach")
	return nil
}
