// Package hooks is a fixture-bound suite that records every hook it runs and
// misbehaves at one of them, named by HOOKS_AT, in the way HOOKS_MODE says:
// error (a fixture hook returns one), assert (a suite hook fails an
// assertion), panic, overrun (the hook sleeps a second, past the suite's
// 300ms Timeout) or wait (the hook waits on its context, and marks that it
// saw it end).
package hooks

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	markMu sync.Mutex
	firsts = map[string]bool{}
)

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

// misbehaves reports whether this hook is the one told to go wrong, once.
func misbehaves(hook string) bool {
	if os.Getenv("HOOKS_AT") != hook {
		return false
	}
	markMu.Lock()
	defer markMu.Unlock()
	if firsts[hook] {
		return false
	}
	firsts[hook] = true
	return true
}

// fixtureHook marks the hook and goes wrong when told to; a fixture hook can
// return an error, panic, overrun or wait.
func fixtureHook(ctx context.Context, hook string) error {
	mark(hook)
	if !misbehaves(hook) {
		return nil
	}
	switch os.Getenv("HOOKS_MODE") {
	case "panic":
		panic(hook + " panicked")
	case "overrun":
		time.Sleep(time.Second)
		return nil
	case "wait":
		waitOn(ctx, hook)
		return nil
	default:
		return errors.New(hook + " failed")
	}
}

// waitOn waits for ctx to end and marks that it did, or gives up after a
// while, past any Timeout here.
func waitOn(ctx context.Context, hook string) {
	select {
	case <-ctx.Done():
		mark(hook + " saw its context end")
	case <-time.After(3 * time.Second):
	}
}

// GridFixture is the package fixture every hook row goes through.
type GridFixture struct{}

func (f *GridFixture) BeforeAll(ctx context.Context) error {
	return fixtureHook(ctx, "fixture.BeforeAll")
}

func (f *GridFixture) AfterAll(ctx context.Context) error {
	return fixtureHook(ctx, "fixture.AfterAll")
}

func (f *GridFixture) BeforeEach(ctx context.Context) error {
	return fixtureHook(ctx, "fixture.BeforeEach")
}

func (f *GridFixture) AfterEach(ctx context.Context) error {
	return fixtureHook(ctx, "fixture.AfterEach")
}
