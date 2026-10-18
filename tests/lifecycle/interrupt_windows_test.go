//go:build windows

package lifecycle_test

import (
	"errors"
	"os/exec"
)

// statusControlCExit is what a console process reports when a control event
// ended it, and what the runtime exits with after its teardown.
const statusControlCExit = 0xC000013A

// endedOnItsOwn reports whether a waited-for process ended with a status of
// its own rather than the console interrupt's: a stopped run finishes its
// tests and teardown and exits as the testing package does.
func endedOnItsOwn(err error) bool {
	if err == nil {
		return true
	}
	var exit *exec.ExitError
	return errors.As(err, &exit) && uint32(exit.ExitCode()) != statusControlCExit //nolint:gosec // G115: a Windows exit status is a DWORD
}

// stopSignalsTheSuite: the runner sends a suite nothing after its stop file.
const stopSignalsTheSuite = false
