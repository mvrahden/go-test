//go:build !unix

package gotestruntime

// No process group to end here: on Windows the runner's job ends what the
// tests started.
func endGroup() {}
