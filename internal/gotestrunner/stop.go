package gotestrunner

// stopFileSignal is the stop where no signal reaches one process alone: the
// stop file the caller just wrote is the signal, which the suite's watcher
// polls for once it has published its pid.
func stopFileSignal(int, int) error { return nil }
