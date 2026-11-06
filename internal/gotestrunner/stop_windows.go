//go:build windows

package gotestrunner

import (
	"errors"

	"golang.org/x/sys/windows"
)

// signalSuite stops a suite by its stop file alone on Windows: no console
// event reaches one process alone, only every process on the suite's console.
var signalSuite = stopFileSignal

// stopAgain has no second stop to send on Windows: the stop file was the
// first, and a console event would reach every process on the console.
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
