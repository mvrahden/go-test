//go:build !unix

package gotestruntime

import "os"

// No death signal here: the runner's job ends the suite process with it.
var runnerDeathSignal os.Signal

// No process group to end here: on Windows the runner's job ends what the
// tests started.
func endGroup() {}
