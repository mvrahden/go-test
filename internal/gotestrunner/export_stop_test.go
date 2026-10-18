package gotestrunner

// ExportUseStopFileSignal makes a cancelled run stop its suite by the stop
// file alone, as on Windows, and returns the call that restores the default.
func ExportUseStopFileSignal() (restore func()) {
	prev := signalSuite
	signalSuite = stopFileSignal
	return func() { signalSuite = prev }
}
