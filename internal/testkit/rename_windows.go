//go:build windows

package testkit

import (
	"errors"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// renameDir moves the dir from to to. Windows refuses while any file in it
// is open, as a scanner or a judging sweep has one for a moment, so it tries
// again for a while.
func renameDir(from, to string) error {
	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		err := os.Rename(from, to)
		if err == nil || time.Now().After(deadline) ||
			!errors.Is(err, windows.ERROR_ACCESS_DENIED) && !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
			return err
		}
		time.Sleep(10 * time.Millisecond)
	}
}
