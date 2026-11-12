//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package gotestruntime

import "os"

// No death signal here: the channel's end alone tells of the runner's.
var runnerDeathSignal os.Signal

// No cheap look at the group's members here: the group gets its whole grace.
func othersInGroup(int) bool { return true }
