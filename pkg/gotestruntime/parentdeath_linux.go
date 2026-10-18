//go:build linux

package gotestruntime

import (
	"os"
	"syscall"
)

func init() { askForParentDeath() }

// askForParentDeath has the kernel send this process SIGTERM when its parent
// is gone, under the runner only. The runner's own death signal reaches the
// process it started, the converter or an -exec wrapper, which may die
// without passing anything on; this process then stops by the watcher's
// parent-gone rule and tears down. A parent gone before the request took
// hold gets the same signal by hand. It runs during package init, on the
// main thread, which lives as long as the process.
func askForParentDeath() {
	if stopFile == "" {
		return
	}
	if _, _, errno := syscall.RawSyscall(syscall.SYS_PRCTL, syscall.PR_SET_PDEATHSIG, uintptr(syscall.SIGTERM), 0); errno != 0 {
		return
	}
	if os.Getppid() != parentPID {
		_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
	}
}
