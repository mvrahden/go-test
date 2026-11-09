//go:build unix

// Package channel checks, from inside a suite process under the runner, that
// the runner's channel stays with that process: a process a test starts
// inherits none of it, a descriptor named as the channel that is none is
// left open, and the last suite leaves no reader of it behind.
package channel

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
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

// probe runs in a process a test started from this binary: it marks the
// sockets it inherited, or writes to the descriptor 3 it was handed.
func probe(kind string) {
	switch kind {
	case "inherited":
		mark(fmt.Sprintf("sockets=%d", countSockets()))
	case "descriptor":
		if _, err := os.NewFile(3, "fd3").WriteString("kept\n"); err != nil {
			mark("descriptor lost: " + err.Error())
		}
	}
}

// readerLeft reports whether a goroutine still reads the runner's channel.
func readerLeft() bool {
	buf := make([]byte, 1<<20)
	return strings.Contains(string(buf[:runtime.Stack(buf, true)]), "gotestruntime.(*watcher).read")
}

// countSockets counts the open descriptors that are sockets.
func countSockets() int {
	n := 0
	for fd := 0; fd < 4096; fd++ {
		var st syscall.Stat_t
		if syscall.Fstat(fd, &st) == nil && st.Mode&syscall.S_IFMT == syscall.S_IFSOCK {
			n++
		}
	}
	return n
}
