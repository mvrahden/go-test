//go:build unix

package filewrite

import (
	"os"
	"syscall"
)

// renameKeeps reports whether a new file renamed over target keeps what
// target has: no other link to it, our ownership, no extended attributes.
func renameKeeps(target string, info os.FileInfo) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && st.Nlink == 1 && int(st.Uid) == os.Geteuid() && !hasXattrs(target)
}

// sameGroup gives t the group info has, as far as we may.
func sameGroup(t *os.File, info os.FileInfo) bool {
	want := info.Sys().(*syscall.Stat_t).Gid
	ti, err := t.Stat()
	if err != nil {
		return false
	}
	if ti.Sys().(*syscall.Stat_t).Gid == want {
		return true
	}
	return t.Chown(-1, int(want)) == nil
}
