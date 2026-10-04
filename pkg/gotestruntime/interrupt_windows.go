//go:build windows

package gotestruntime

import (
	"os"
	"syscall"
)

var stopSignals = []os.Signal{syscall.SIGTERM}

func ignoreBrokenPipe() {}

// statusControlCExit is the status a console process reports when a control
// event ends it; the runner reads it as a process stopped from outside.
const statusControlCExit = 0xC000013A

func reraise(os.Signal) { os.Exit(statusControlCExit) }
