// Package filewrite rewrites a set of files all or none: every file is
// checked and its new contents readied before the first one is written.
package filewrite

import (
	"bytes"
	"errors"
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
}

type staged struct {
	file, target string
	after        []byte
	tmp          string // the replacement renamed over target; "" writes in place
}

// Stage checks that every file can be written and still holds Before, and
// readies its new contents; it writes none of them. Each file is replaced
// by renaming a temporary file beside it over it when that keeps all it
// has: one name, the owner, the group and no extended attributes (on
// Windows, one name). Otherwise, as when its directory cannot be written, it
// is written in place. The file a symlink names is the one rewritten, so
// the link stays. On an error nothing is left behind.
func Stage(changes []Change) (*Staged, error) {
	s := &Staged{}
	for _, c := range changes {
		st, err := stage(c)
		if err != nil {
			s.Discard()
			return nil, err
		}
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
	st := staged{file: c.File, target: target, after: c.After}
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

// Commit writes every staged file in turn and returns how many it wrote,
// in the order of the changes. On an error it stops there; a file written
// in place may be left partly written, and the error says so.
func (s *Staged) Commit() (int, error) {
	for i, st := range s.files {
		if err := st.write(); err != nil {
			s.files = s.files[i:]
			s.Discard()
			return i, err
		}
	}
	n := len(s.files)
	s.files = nil
	return n, nil
}

func (st *staged) write() error {
	if st.tmp != "" {
		return os.Rename(st.tmp, st.target)
	}
	f, err := os.OpenFile(st.target, os.O_WRONLY|os.O_TRUNC, 0)
	if err != nil {
		return err
	}
	_, err = f.Write(st.after)
	err = errors.Join(err, f.Close())
	if err != nil {
		return fmt.Errorf("%w; %s may be partly written", err, st.file)
	}
	return nil
}

// Discard removes what Stage readied and writes nothing.
func (s *Staged) Discard() {
	for _, st := range s.files {
		if st.tmp != "" {
			_ = os.Remove(st.tmp)
		}
	}
	s.files = nil
}
