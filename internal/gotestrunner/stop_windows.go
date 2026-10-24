//go:build windows

package gotestrunner

// signalSuite stops a suite by its stop file alone on Windows: a console
// event would reach the converter in front of it, which cannot ignore one
// there and dies, taking the rest of the stream with it.
var signalSuite = stopFileSignal
