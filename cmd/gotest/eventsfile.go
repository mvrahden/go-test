package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// openEventsFile opens the file --events-file names for the run's event
// stream, making its directories and replacing a file already there; nil
// without the flag. It runs before anything else does, so an unwritable
// path fails at once.
func openEventsFile(own OwnFlags) (*os.File, error) {
	path := own.Value("--events-file")
	if path == "" {
		return nil, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("--events-file: %w", err)
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("--events-file: %w", err)
	}
	return f, nil
}

// eventsWriter is f as the run's events writer: nil, not a nil *os.File in
// an interface, without the flag.
func eventsWriter(f *os.File) io.Writer {
	if f == nil {
		return nil
	}
	return f
}
