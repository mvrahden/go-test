//go:build unix && !linux

package gotestruntime

// No cheap look at the group's members here: the group gets its whole grace.
func othersInGroup(int) bool { return true }
