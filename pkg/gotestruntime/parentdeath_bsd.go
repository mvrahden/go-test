//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package gotestruntime

import (
	"sync/atomic"
	"syscall"
)

// parentQueue reports the exit of parentPID, under the runner only; -1
// otherwise. It is set up during init, while the parent still holds its pid.
var parentQueue = watchParent()

var parentGone atomic.Bool

func watchParent() int {
	if stopFile == "" {
		return -1
	}
	kq, err := syscall.Kqueue()
	if err != nil {
		return -1
	}
	syscall.CloseOnExec(kq)
	var ev syscall.Kevent_t
	syscall.SetKevent(&ev, parentPID, syscall.EVFILT_PROC, syscall.EV_ADD|syscall.EV_ONESHOT)
	ev.Fflags = syscall.NOTE_EXIT
	if _, err := syscall.Kevent(kq, []syscall.Kevent_t{ev}, nil, nil); err != nil {
		_ = syscall.Close(kq)
		if err == syscall.ESRCH {
			parentGone.Store(true)
		}
		return -1
	}
	return kq
}

// parentExited reports whether the process that started this one has ended:
// the watcher's poll asks, as no death signal comes here.
func parentExited() bool {
	if parentGone.Load() {
		return true
	}
	if parentQueue < 0 {
		return false
	}
	var evs [1]syscall.Kevent_t
	if n, err := syscall.Kevent(parentQueue, nil, evs[:], &syscall.Timespec{}); err == nil && n > 0 {
		parentGone.Store(true)
	}
	return parentGone.Load()
}
