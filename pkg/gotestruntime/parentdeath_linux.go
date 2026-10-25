//go:build linux

package gotestruntime

import (
	"os"
	"syscall"
)

func init() { askForParentDeath() }

// deathSignal is what the kernel sends this process when its parent is gone.
// It is no stop signal, so nobody's stop request is ever taken for it: it
// only wakes the watcher to look at the parent. A process started with it
// ignored never gets it, and the watcher's poll sees the parent go instead.
var deathSignal os.Signal = syscall.SIGHUP

// askForParentDeath has the kernel send this process deathSignal when its
// parent is gone, under the runner only, in place of any death signal it was
// started with. The runner's own death signal reaches the process it started,
// the converter or an -exec wrapper, which may die without passing anything
// on; this process then stops by the watcher's parent-gone rule and tears
// down. A parent gone before the request took hold gets the same signal by
// hand. It runs during package init, on the main thread, which lives as long
// as the process.
func askForParentDeath() {
	if stopFile == "" {
		return
	}
	if _, _, errno := syscall.RawSyscall(syscall.SYS_PRCTL, syscall.PR_SET_PDEATHSIG, uintptr(syscall.SIGHUP), 0); errno != 0 {
		return
	}
	if os.Getppid() != parentPID {
		_ = syscall.Kill(os.Getpid(), syscall.SIGHUP)
	}
}

// parentExited reports whether the process that started this one has ended:
// an orphan is handed to another parent.
func parentExited() bool { return os.Getppid() != parentPID }
