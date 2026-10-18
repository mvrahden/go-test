//go:build !linux && !darwin

package gotestruntime

// No check for a reader that is gone here.
func stdoutReaderGone() bool { return false }
