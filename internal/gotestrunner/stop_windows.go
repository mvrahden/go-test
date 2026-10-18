//go:build windows

package gotestrunner

import "errors"

// signalSuite has no signal for one process on Windows; the caller falls back
// to the console interrupt the tree sends.
func signalSuite(int) error { return errors.New("no per-process signal on windows") }
