//go:build unix && !(linux || darwin || freebsd || netbsd)

package filewrite

// hasXattrs is false where the system offers no extended attributes to
// list.
func hasXattrs(string) bool { return false }
