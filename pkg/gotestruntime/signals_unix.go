//go:build unix

package gotestruntime

import (
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

var stopSignals = []os.Signal{syscall.SIGTERM}

var pipeOnce sync.Once

// notePipe keeps a write to a reader that already exited — test2json ends on
// the runner's group SIGTERM before this process does — from killing the
// process: with SIGPIPE delivered to a channel, Go returns EPIPE instead.
// Unlike an ignored SIGPIPE, a handled one is reset across exec, so processes a
// test starts keep the default.
func notePipe() {
	pipeOnce.Do(func() { signal.Notify(make(chan os.Signal, 1), syscall.SIGPIPE) })
}

// reraise ends the process by sig, so it reports what it would have without
// the teardown. A process started with sig ignored gets that back from Reset
// and survives the kill; it exits with the status a shell gives for sig.
func reraise(sig os.Signal) {
	s, ok := sig.(syscall.Signal)
	if !ok {
		os.Exit(1)
	}
	signal.Reset(sig)
	_ = syscall.Kill(syscall.Getpid(), s)
	time.Sleep(time.Second)
	os.Exit(128 + int(s))
}
