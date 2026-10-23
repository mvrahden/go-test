// Package hookoverrun has suites whose fixture's BeforeEach (ahead of
// another fixture's) or AfterEach sleeps past the Timeout; the canary reads
// what that does to the method and where it is reported.
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
	time.Sleep(time.Second)
	return nil
}

// SlowReleaseFixture takes longer to release a method than the suite allows.
type SlowReleaseFixture struct{}

func (f *SlowReleaseFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *SlowReleaseFixture) AfterAll(ctx context.Context) error  { return nil }

func (f *SlowReleaseFixture) AfterEach(ctx context.Context) error {
	time.Sleep(time.Second)
	return nil
}

// WaitingFixture waits on its context to prepare a method, and marks
// whether that context ended or it gave up.
type WaitingFixture struct{}

func (f *WaitingFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *WaitingFixture) AfterAll(ctx context.Context) error  { return nil }

func (f *WaitingFixture) BeforeEach(ctx context.Context) error {
	waitOn(ctx, "fixture")
	return nil
}

// waitOn waits for ctx to end and marks that who saw it, or gives up after
// three seconds, past every Timeout here.
func waitOn(ctx context.Context, who string) {
	select {
	case <-ctx.Done():
		mark(who + " saw its context end")
	case <-time.After(3 * time.Second):
		mark(who + " gave up")
	}
}

// LaterFixture comes after SlowFixture; its BeforeEach marks that it
// started and fails once the method's context has ended.
type LaterFixture struct {
	Slow *SlowFixture
}

func (f *LaterFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *LaterFixture) AfterAll(ctx context.Context) error  { return nil }

func (f *LaterFixture) BeforeEach(ctx context.Context) error {
	mark("later fixture BeforeEach")
	return ctx.Err()
}
