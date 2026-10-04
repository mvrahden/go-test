//go:build windows

package canary_test

import (
	"errors"
	"os/exec"
)

// statusControlCExit is what a console process reports when a control event
// ended it, and what the runtime exits with after its teardown.
const statusControlCExit = 0xC000013A

// stoppedFromOutside reports whether a waited-for process ended with the
// status of a console control event.
func stoppedFromOutside(err error) bool {
	var exit *exec.ExitError
	return errors.As(err, &exit) && uint32(exit.ExitCode()) == statusControlCExit //nolint:gosec // G115: a Windows exit status is a DWORD
}
