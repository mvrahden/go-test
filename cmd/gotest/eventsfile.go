package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/internal/protocol"
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

// closeEvents closes the events file, when there is one.
func closeEvents(f *os.File) {
	if f != nil {
		_ = f.Close()
	}
}

// liveStream reports whether a run writes each suite's events as its process
// writes them: --live, else GOTEST_LIVE, only for a -json stream. --live
// without -json is refused; the variable outside one is not read.
func liveStream(own OwnFlags, jsonMode bool, getenv func(string) string) (bool, error) {
	if own.Given("--live") {
		on := own.Bool("--live")
		if on && !jsonMode {
			return false, errors.New("--live writes the go test -json stream as each suite process writes it, which a text run does not produce; add -json")
		}
		return on, nil
	}
	return jsonMode && protocol.SwitchOn(getenv(protocol.EnvLive)), nil
}
