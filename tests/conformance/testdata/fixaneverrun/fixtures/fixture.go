// Package fixtures holds the shared fixture two suites read, one of whose
// packages never links.
package fixtures

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var markMu sync.Mutex

// Mark appends one line to the markers file the run under test reads back.
func Mark(event string) {
	markMu.Lock()
	defer markMu.Unlock()
	f, err := os.OpenFile(filepath.Join(os.Getenv("GOTEST_CANARY_DIR"), "markers"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(event + "\n")
}

// Await waits until event is marked, or for limit, and says whether it was.
func Await(event string, limit time.Duration) bool {
	for end := time.Now().Add(limit); time.Now().Before(end); time.Sleep(50 * time.Millisecond) {
		data, _ := os.ReadFile(filepath.Join(os.Getenv("GOTEST_CANARY_DIR"), "markers"))
		if strings.Contains(string(data), event+"\n") {
			return true
		}
	}
	return false
}

type ASharedFixture struct {
	State string
}

func (f *ASharedFixture) BeforeAll(ctx context.Context) error {
	f.State = "up"
	return nil
}

func (f *ASharedFixture) AfterAll(ctx context.Context) error {
	Mark("a.AfterAll")
	return nil
}
