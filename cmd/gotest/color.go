package main

import (
	"fmt"
	"os"
)

// stdoutIsTerminal reports whether stdout is a character device, the one
// place color codes mean anything. Tests swap it.
var stdoutIsTerminal = func() bool {
	fi, err := os.Stdout.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// colorDisabled decides color for human output: off when --no-color was
// given, when NO_COLOR is set (https://no-color.org) or when stdout is not a
// terminal, so a log, a file or a pipe never receives escape codes.
func colorDisabled(noColorFlag bool) bool {
	if noColorFlag || os.Getenv("NO_COLOR") != "" {
		return true
	}
	return !stdoutIsTerminal()
}

// checkFormat validates a --format value before any work is done.
func checkFormat(format string) error {
	switch format {
	case "terminal", "md", "markdown", "json":
		return nil
	}
	return fmt.Errorf("unknown --format %q: terminal, md or json", format)
}
