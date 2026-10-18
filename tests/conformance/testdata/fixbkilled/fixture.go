// Package fixbkilled binds a shared fixture whose process is left behind when
// the CLI is killed. FIXB_TIMEOUT sets the fixture's Timeout, FIXB_SETUP=wait
// holds BeforeAll until its context ends, FIXB_SETUP=ignore holds it for good,
// and FIXB_TEARDOWN=hang keeps AfterAll from returning.
package fixbkilled

import (
	"context"
	"fmt"
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

// KeepSharedFixture marks its process, its setup and its release.
type KeepSharedFixture struct {
	State string
}

func (f *KeepSharedFixture) SharedFixtureConfig() gotest.FixtureConfig {
	cfg := gotest.DefaultFixtureConfig()
	if d, err := time.ParseDuration(os.Getenv("FIXB_TIMEOUT")); err == nil {
		cfg.Timeout = d
	}
	return cfg
}

func (f *KeepSharedFixture) BeforeAll(ctx context.Context) error {
	mark(fmt.Sprintf("pid=%d", os.Getpid()))
	switch os.Getenv("FIXB_SETUP") {
	case "wait":
		mark("setup waiting")
		<-ctx.Done()
		mark("setup context ended")
		return ctx.Err()
	case "ignore":
		mark("setup ignoring")
		time.Sleep(time.Hour)
	}
	f.State = "up"
	mark("up")
	return nil
}

func (f *KeepSharedFixture) AfterAll(ctx context.Context) error {
	mark("AfterAll begins")
	if os.Getenv("FIXB_TEARDOWN") == "hang" {
		time.Sleep(time.Hour)
	}
	mark("AfterAll ends")
	return nil
}
