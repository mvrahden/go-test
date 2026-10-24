//go:build windows

package proctree

import "golang.org/x/sys/windows"

var ExportInterruptConsole = interruptConsole

// ExportFailOpenProcess makes every process lookup fail with err, and
// returns what restores the real one.
func ExportFailOpenProcess(err error) (restore func()) {
	openProcess = func(uint32, bool, uint32) (windows.Handle, error) { return 0, err }
	return func() { openProcess = windows.OpenProcess }
}
