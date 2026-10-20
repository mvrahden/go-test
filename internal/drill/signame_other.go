//go:build !linux

package drill

func signalName(n int) string { return "signal " + itoa(n) }
