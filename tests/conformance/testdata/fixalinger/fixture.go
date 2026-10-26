// Package fixalinger has a suite that leaves a process of its own running
// past its end, while it reads a shared fixture.
package fixalinger

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
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

type ASharedFixture struct {
	State string
}

func (f *ASharedFixture) BeforeAll(ctx context.Context) error {
	f.State = "up"
	return nil
}

// AfterAll marks whether the process the suite left behind still runs.
func (f *ASharedFixture) AfterAll(ctx context.Context) error {
	data, _ := os.ReadFile(filepath.Join(os.Getenv("GOTEST_CANARY_DIR"), "markers"))
	for _, line := range strings.Split(string(data), "\n") {
		if pid, ok := strings.CutPrefix(line, "child="); ok {
			if n, err := strconv.Atoi(pid); err == nil && alive(n) {
				mark("a.AfterAll with the child running")
				return nil
			}
		}
	}
	mark("a.AfterAll")
	return nil
}
