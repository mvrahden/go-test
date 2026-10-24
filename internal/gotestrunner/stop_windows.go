//go:build windows

package gotestrunner

import "errors"

// signalSuite stops a suite by its stop file alone on Windows: a console
// event would reach the converter in front of it, which cannot ignore one
// there and dies, taking the rest of the stream with it.
var signalSuite = stopFileSignal

// stopAgain has no second stop to send on Windows: the stop file was the
// first, and a console event would reach the converter too.
func stopAgain(int, int) error { return errNoSecondStop }

var errNoSecondStop = errors.New("no second stop reaches a suite process alone")
