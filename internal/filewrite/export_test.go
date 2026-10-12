package filewrite

import "os"

// SetRename replaces the rename s commits through.
func SetRename(s *Staged, rename func(oldpath, newpath string) error) { s.rename = rename }

// SetWriteAt replaces the in-place write s commits through.
func SetWriteAt(s *Staged, writeAt func(f *os.File, b []byte, off int64) (int, error)) {
	s.writeAt = writeAt
}
