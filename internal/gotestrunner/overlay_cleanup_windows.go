//go:build windows

package gotestrunner

func processAlive(_ int) bool {
	// On Windows, there is no reliable equivalent to Unix signal-zero probing
	// without pulling in additional dependencies. Conservatively assume alive;
	// stale overlays get overwritten on the next run that owns the directory.
	return true
}
