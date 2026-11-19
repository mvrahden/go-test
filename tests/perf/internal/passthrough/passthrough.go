// Package passthrough is what the perf suites' stand-in commands share:
// append a line to a log, then become another program.
package passthrough

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// Log appends line to the file at path in one write, so concurrent writers
// never interleave within a line.
func Log(path, line string) error {
	if path == "" {
		return errors.New("no log file named")
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // G304: the log the caller names
	if err != nil {
		return err
	}
	if _, err := f.WriteString(line); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// Run replaces this process with program where the platform can, so its
// CPU, signals and exit status are the program's own; elsewhere it runs it
// as a child with the same streams. It returns the exit code to end with.
func Run(program string, args []string) int {
	path, err := exec.LookPath(program)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	// Exec returns only when it failed or is not supported (Windows).
	_ = syscall.Exec(path, append([]string{program}, args...), os.Environ()) //nolint:gosec // G204: the program the caller names

	cmd := exec.Command(path, args...) //nolint:gosec // G204: the program the caller names
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() >= 0 {
			return exit.ExitCode()
		}
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
