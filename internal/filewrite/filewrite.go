// Package filewrite rewrites a set of files all or none: every file is
// checked and its new contents readied before the first one is written.
package filewrite

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
)

// Change is one file to rewrite.
type Change struct {
	File   string
	Before []byte // the file on disk
	After  []byte // what is written
}

// Staged is a set of changes ready to write.
type Staged struct {
	files []staged
	// The calls Commit writes through, replaced by tests.
	rename  func(oldpath, newpath string) error
	writeAt func(f *os.File, b []byte, off int64) (int, error)
}

type staged struct {
	file, target  string
	also          []string // the other names a change reached this file by
	before, after []byte
	tmp           string // the replacement renamed over target; "" writes in place
}

// Stage checks that every file can be written and still holds Before, and
// readies its new contents; it writes none of them. Each file is replaced
// by renaming a temporary file beside it over it when that keeps all it
// has: one name, the owner, the group and no extended attributes (on
// Windows, one name). Otherwise, as when its directory cannot be written, it
// is written in place. The file a symlink names is the one rewritten, so
// the link stays; two changes reaching one file must agree. On an error
// nothing is left behind.
func Stage(changes []Change) (*Staged, error) {
	s := &Staged{rename: os.Rename, writeAt: (*os.File).WriteAt}
	seen := map[string]Change{}
	for _, c := range changes {
		st, err := stage(c)
		if err == nil {
			if prev, ok := seen[st.target]; ok {
				_ = st.discard()
				if bytes.Equal(prev.After, c.After) {
					for i := range s.files {
						if s.files[i].target == st.target {
							s.files[i].also = append(s.files[i].also, c.File)
						}
					}
					continue
				}
				err = fmt.Errorf("%s and %s are one file, changed two ways", prev.File, c.File)
			}
		}
		if err != nil {
			s.Discard()
			return nil, err
		}
		seen[st.target] = c
		s.files = append(s.files, st)
	}
	return s, nil
}

func stage(c Change) (staged, error) {
	target, err := filepath.EvalSymlinks(c.File)
	if err != nil {
		return staged{}, err
	}
	info, err := os.Stat(target)
	if err != nil {
		return staged{}, err
	}
	// Neither a rename nor a write may replace a file its permissions
	// keep from being written.
	f, err := os.OpenFile(target, os.O_WRONLY, 0)
	if err != nil {
		return staged{}, err
	}
	_ = f.Close()
	if disk, err := os.ReadFile(target); err != nil || !bytes.Equal(disk, c.Before) {
		return staged{}, fmt.Errorf("%s changed since it was read", c.File)
	}
	st := staged{file: c.File, target: target, before: c.Before, after: c.After}
	if !renameKeeps(target, info) {
		return st, nil
	}

	t, err := os.CreateTemp(filepath.Dir(target), "."+filepath.Base(target)+".gotest-*")
	if err != nil {
		return st, nil // the directory takes no new file: write in place
	}
	_, err = t.Write(c.After)
	// The group goes first: a chown clears the set-id bits.
	keep := err == nil && sameGroup(t, info)
	if keep {
		err = t.Chmod(info.Mode() & (os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky))
	}
	if cerr := t.Close(); err == nil {
		err = cerr
	}
	if err != nil || !keep {
		_ = os.Remove(t.Name())
		return st, err
	}
	st.tmp = t.Name()
	return st, nil
}

// PartlyWrittenError is a file written in place that failed midway and
// could not get its old contents back.
type PartlyWrittenError struct {
	File string
	Err  error
}

func (e *PartlyWrittenError) Error() string {
	return fmt.Sprintf("%v; %s may be partly written", e.Err, e.File)
}

func (e *PartlyWrittenError) Unwrap() error { return e.Err }

// Commit writes every staged file in turn, in the order of the changes, and
// returns the names of the changes it wrote. On an error it stops there. A
// file written in place that fails midway gets its old contents back; only
// when that fails too is it left partly written, a *PartlyWrittenError.
func (s *Staged) Commit() ([]string, error) {
	var written []string
	for i := range s.files {
		if err := s.write(&s.files[i]); err != nil {
			s.files = s.files[i:]
			s.Discard()
			return written, err
		}
		written = append(append(written, s.files[i].file), s.files[i].also...)
	}
	s.files = nil
	return written, nil
}

func (s *Staged) write(st *staged) error {
	if st.tmp != "" {
		err := s.rename(st.tmp, st.target)
		// A mount point, or a file bound in from another filesystem,
		// takes no rename; it is written in place.
		if err == nil || !renameRefused(err) {
			return err
		}
		if err := st.discard(); err != nil {
			return err
		}
	}
	f, err := os.OpenFile(st.target, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	// Overwritten, not truncated first, so the old contents fit back into
	// the blocks the file already has.
	_, err = s.writeAt(f, st.after, 0)
	if err == nil {
		err = f.Truncate(int64(len(st.after)))
	}
	if err != nil {
		_, rerr := s.writeAt(f, st.before, 0)
		if rerr == nil {
			rerr = f.Truncate(int64(len(st.before)))
		}
		_ = f.Close()
		if rerr != nil {
			return &PartlyWrittenError{File: st.file, Err: err}
		}
		return fmt.Errorf("%w; %s is left as it was", err, st.file)
	}
	return f.Close()
}

func (st *staged) discard() error {
	if st.tmp == "" {
		return nil
	}
	err := os.Remove(st.tmp)
	st.tmp = ""
	return err
}

// Discard removes what Stage readied and writes nothing.
func (s *Staged) Discard() {
	for i := range s.files {
		_ = s.files[i].discard()
	}
	s.files = nil
}
