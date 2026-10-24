//go:build !windows

package main

import (
	"os"
	"syscall"
)

var shutdownSignals = []os.Signal{os.Interrupt, syscall.SIGTERM}

// abandonMessage says what a second signal does, given the abandon bound.
const abandonMessage = "interrupted again: the fixtures are released at once; what still runs %s from now is killed (interrupt again to kill it now)\n"
