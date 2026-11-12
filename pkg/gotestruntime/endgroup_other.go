//go:build !(darwin || dragonfly || freebsd || linux || netbsd || openbsd)

package gotestruntime

import "os"

// No death signal here: the runner's job ends the suite process with it.
var runnerDeathSignal os.Signal

// No process group to end here: on Windows the runner's job ends what the
// tests started, and syscall has no Getpgid on aix and solaris.
func endGroup() {}
