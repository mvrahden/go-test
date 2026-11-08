//go:build windows

package gotestrunner

import (
	"errors"

	"golang.org/x/sys/windows"
)

// signalSuite stops a suite by its stop file alone on Windows: a console
// event would reach the converter in front of it, which cannot ignore one
// there and dies, taking the rest of the stream with it.
var signalSuite = stopFileSignal

// stopAgain has no second stop to send on Windows: the stop file was the
// first, and a console event would reach the converter too.
func stopAgain(int, int) error { return errNoSecondStop }

var errNoSecondStop = errors.New("no second stop reaches a suite process alone")

// killPID kills the suite process pid alone. Windows reuses no pid while a
// handle to its process is open, and the suite's parent holds one until it
// has waited for the suite.
func killPID(pid, _ int) error {
	h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, uint32(pid)) //nolint:gosec // G115: a Windows process ID is a DWORD
	if err != nil {
		return err
	}
	defer func() { _ = windows.CloseHandle(h) }()
	return windows.TerminateProcess(h, 1)
}
