// Package hooks is a fixture-bound suite that records every hook it runs and
// misbehaves at one of them, named by HOOKS_AT, in the way HOOKS_MODE says:
// error (a fixture hook returns one), assert (a suite hook fails an
// assertion), panic, or overrun (the hook sleeps four seconds, past the
// suite's one-second Timeout).
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
// return an error, panic or overrun.
func fixtureHook(hook string) error {
	mark(hook)
	if !misbehaves(hook) {
		return nil
	}
	switch os.Getenv("HOOKS_MODE") {
	case "panic":
		panic(hook + " panicked")
	case "overrun":
		time.Sleep(4 * time.Second)
		return nil
	default:
		return errors.New(hook + " failed")
	}
}

// GridFixture is the package fixture every hook row goes through.
type GridFixture struct{}

func (f *GridFixture) BeforeAll(ctx context.Context) error  { return fixtureHook("fixture.BeforeAll") }
func (f *GridFixture) AfterAll(ctx context.Context) error   { return fixtureHook("fixture.AfterAll") }
func (f *GridFixture) BeforeEach(ctx context.Context) error { return fixtureHook("fixture.BeforeEach") }
func (f *GridFixture) AfterEach(ctx context.Context) error  { return fixtureHook("fixture.AfterEach") }
