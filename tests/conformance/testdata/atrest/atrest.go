// Package atrest counts, from inside a suite process, the heap allocations
// the process makes while its test only waits.
package atrest

import (
	"os"
	"path/filepath"
	"runtime"
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

// mallocsDuring counts the process's heap allocations while the caller only
// sleeps for d.
func mallocsDuring(d time.Duration) uint64 {
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	time.Sleep(d)
	runtime.ReadMemStats(&after)
	return after.Mallocs - before.Mallocs
}
