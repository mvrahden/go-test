// Package groups has suites in the default group and in two named groups;
// each stamps when it begins and ends.
package groups

import (
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

var markMu sync.Mutex

// mark appends one line to the markers file the run under test reads back:
// the event and when it happened, in Unix milliseconds.
func mark(event string) {
	markMu.Lock()
	defer markMu.Unlock()
	f, err := os.OpenFile(filepath.Join(os.Getenv("GOTEST_CANARY_DIR"), "markers"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(event + "@" + strconv.FormatInt(time.Now().UnixMilli(), 10) + "\n")
}

// span stamps name's begin, holds for d, and stamps its end.
func span(name string, d time.Duration) {
	mark(name + " begins")
	time.Sleep(d)
	mark(name + " ends")
}
