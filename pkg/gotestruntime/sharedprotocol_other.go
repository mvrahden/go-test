//go:build !unix

package gotestruntime

// The runner passes no protocol descriptor here.
func closeOnExec(int) {}
