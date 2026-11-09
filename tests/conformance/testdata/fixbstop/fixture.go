// Package fixbstop binds a shared fixture the runner asks to stop. The
// fixture marks its process and the stop signals it got before its release;
// FIXB_RELEASE=wait holds its AfterAll until its context ends, for 20s at
// most.
package fixbstop

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
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

// signals holds the stop signals the fixture process gets until its release.
var signals = make(chan os.Signal, 2)

// markSignals marks the stop signals the process got. Stop returns only once
// a signal being handed out has reached every channel, so a signal that
// stopped the process is marked here; a goroutine marking it could lose the
// race with the process's exit.
func markSignals() {
	signal.Stop(signals)
	for {
		select {
		case s := <-signals:
			mark("signal " + s.String())
		default:
			return
		}
	}
}

// AskedSharedFixture marks its process, the stop signals it got before its
// release, and its release.
type AskedSharedFixture struct {
	State string
}

func (f *AskedSharedFixture) BeforeAll(ctx context.Context) error {
	mark(fmt.Sprintf("pid=%d", os.Getpid()))
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)
	f.State = "up"
	return nil
}

func (f *AskedSharedFixture) AfterAll(ctx context.Context) error {
	markSignals()
	mark("AfterAll begins")
	if os.Getenv("FIXB_RELEASE") == "wait" {
		select {
		case <-ctx.Done():
			mark("AfterAll context ended")
		case <-time.After(20 * time.Second):
		}
	}
	mark("AfterAll ends")
	return nil
}

// HoldFixture is a package fixture whose release takes 10s, its context
// ignored: with it, a stopped suite still runs for 10s past a second stop.
type HoldFixture struct{}

func (f *HoldFixture) BeforeAll(ctx context.Context) error { return nil }

func (f *HoldFixture) AfterAll(ctx context.Context) error {
	mark("hold release begins")
	time.Sleep(10 * time.Second)
	return nil
}
