// Package killedcli binds a shared fixture whose process is left behind when
// the CLI is killed while the fixture is up. Its AfterAll logs and takes 2s.
package killedcli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
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

// KeepSharedFixture marks its process and its release.
type KeepSharedFixture struct {
	State string
}

func (f *KeepSharedFixture) BeforeAll(ctx context.Context) error {
	mark(fmt.Sprintf("pid=%d", os.Getpid()))
	f.State = "up"
	return nil
}

// AfterAll writes to the output the dead CLI read, then takes 2s.
func (f *KeepSharedFixture) AfterAll(ctx context.Context) error {
	mark("AfterAll begins")
	for range 20 {
		fmt.Println("releasing")
		fmt.Fprintln(os.Stderr, "releasing")
		time.Sleep(100 * time.Millisecond)
	}
	mark("AfterAll ends")
	return nil
}
