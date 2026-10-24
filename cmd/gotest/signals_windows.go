//go:build windows

package main

import "os"

var shutdownSignals = []os.Signal{os.Interrupt}

// abandonMessage says what a second signal does, given the abandon bound: no
// second stop reaches a suite on Windows, so the suites are killed at once.
const abandonMessage = "interrupted again: the suites are killed now; the shared fixtures get %s to tear down (interrupt again to kill them now)\n"
