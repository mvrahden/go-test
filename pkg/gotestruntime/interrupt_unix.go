//go:build !windows

package gotestruntime

import (
	"os"
	"os/signal"
	"syscall"
	"time"
)

var stopSignals = []os.Signal{syscall.SIGTERM}

// ignoreBrokenPipe keeps writes to a reader that already exited (test2json
// receives the same signal) from killing the process mid-teardown.
func ignoreBrokenPipe() { signal.Ignore(syscall.SIGPIPE) }

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
