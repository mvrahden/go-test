//go:build windows

package gotestruntime

import (
	"os"
	"syscall"
)

var stopSignals = []os.Signal{syscall.SIGTERM}

func notePipe() {}

// statusControlCExit is 0xC000013A, the status of a console process a control
// event ended; the runner reads it as a process stopped from outside. Written
// as the int whose uint32 it is, which fits on 386 too: ExitProcess takes a
// uint32.
const statusControlCExit = -1073741510

func reraise(os.Signal) { os.Exit(statusControlCExit) }
