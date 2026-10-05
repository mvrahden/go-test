//go:build unix && !linux

package proctree

// groupHasLiveMember cannot tell a zombie from a live process here; PID 1
// reaps on these systems, so the group probe is trusted as it is.
func groupHasLiveMember(int) (live, known bool) { return false, false }
