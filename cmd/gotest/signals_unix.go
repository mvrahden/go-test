//go:build !windows

package main

import (
	"os"
	"os/signal"
	"syscall"
)

// shutdownSignals: SIGHUP is the terminal closing, unless it was ignored at
// start, as nohup leaves it: catching it would undo that.
var shutdownSignals = append([]os.Signal{os.Interrupt, syscall.SIGTERM}, unlessIgnored(syscall.SIGHUP)...)

func unlessIgnored(sig os.Signal) []os.Signal {
	if signal.Ignored(sig) {
		return nil
	}
	return []os.Signal{sig}
}

// abandonMessage says what a second signal does, given the abandon bound.
const abandonMessage = "interrupted again: the fixtures are released at once; what still runs %s from now is killed (interrupt again to kill it now)\n"
