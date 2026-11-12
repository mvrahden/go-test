//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package gotestruntime

import (
	"os"
	"os/signal"
	"syscall"
	"time"
)

// groupGrace is how long the rest of the process group gets to exit on the
// SIGTERM before it is killed.
const groupGrace = 2 * time.Second

// endGroup ends this process's group once the runner, which would have, is
// gone: nobody else is left to end what the tests started. SIGTERM to the
// group, up to groupGrace for its other members to exit, then SIGKILL to the
// group, this process with it. This process's own SIGTERM is caught and
// dropped. Group 1 is never signalled: kill(-1) reaches every process.
func endGroup() {
	pgid, err := syscall.Getpgid(0)
	if err != nil || pgid <= 1 {
		return
	}
	signal.Notify(make(chan os.Signal, 1), syscall.SIGTERM)
	_ = syscall.Kill(-pgid, syscall.SIGTERM)
	for deadline := time.Now().Add(groupGrace); time.Now().Before(deadline) && othersInGroup(pgid); {
		time.Sleep(20 * time.Millisecond)
	}
	_ = syscall.Kill(-pgid, syscall.SIGKILL)
	time.Sleep(time.Second)
	os.Exit(128 + int(syscall.SIGKILL))
}
