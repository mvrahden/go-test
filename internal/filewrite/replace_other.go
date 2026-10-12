//go:build !unix && !windows

package filewrite

import "os"

// renameKeeps is always true off Unix and Windows: there is no owner or link
// count a rename could lose that the in-place write would keep.
func renameKeeps(string, os.FileInfo) bool { return true }

func sameGroup(*os.File, os.FileInfo) bool { return true }
