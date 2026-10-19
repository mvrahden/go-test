package gotestrunner

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// FuzzSession is what the latest gotest fuzz session left for promote: the
// crasher files it wrote, each with the digest of its content. Promote takes
// these files and no others.
type FuzzSession struct {
	Crashers []SessionCrasher `json:"crashers"`
}

// SessionCrasher is one crasher file a session wrote.
type SessionCrasher struct {
	Dir    string `json:"dir"`    // package source directory
	Func   string `json:"func"`   // generated fuzz func name
	Name   string `json:"name"`   // file name under testdata/fuzz/<Func>/
	SHA256 string `json:"sha256"` // digest of the content the session left
}

// Path is the crasher file's path.
func (c SessionCrasher) Path() string {
	return filepath.Join(c.Dir, "testdata", "fuzz", c.Func, c.Name)
}

// Check reports whether the file still holds what the session wrote.
func (c SessionCrasher) Check() error {
	sum, err := fileDigest(c.Path())
	if err != nil {
		return err
	}
	if sum != c.SHA256 {
		return errors.New("changed since the fuzz session wrote it")
	}
	return nil
}

// NewSessionCrasher records the crasher file name of fuzz func fn in package
// directory dir with its current content's digest.
func NewSessionCrasher(dir, fn, name string) (SessionCrasher, error) {
	c := SessionCrasher{Dir: dir, Func: fn, Name: name}
	sum, err := fileDigest(c.Path())
	if err != nil {
		return SessionCrasher{}, err
	}
	c.SHA256 = sum
	return c, nil
}

func fileDigest(path string) (string, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: a corpus file under the package's testdata
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// For returns the session's crashers of target t.
func (s *FuzzSession) For(t FuzzTarget) []SessionCrasher { //nolint:gocritic // hugeParam: stable API
	var out []SessionCrasher
	for _, c := range s.Crashers {
		if c.Func == t.Func && sameDir(c.Dir, t.Dir) {
			out = append(out, c)
		}
	}
	return out
}

// sameDir compares two directories by what they resolve to.
func sameDir(a, b string) bool {
	return resolveDir(a) == resolveDir(b)
}

func resolveDir(dir string) string {
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		dir = real
	}
	return filepath.Clean(dir)
}

// FuzzSessionFile is where the session record lives in the gotest cache
// cacheDir.
func FuzzSessionFile(cacheDir string) string {
	return filepath.Join(cacheDir, "fuzz-session.json")
}

// CurrentFuzzSessionFile is the session record of this environment's cache.
func CurrentFuzzSessionFile() (string, error) {
	root, err := cacheRoot()
	if err != nil {
		return "", err
	}
	return FuzzSessionFile(root), nil
}

// WriteFuzzSession replaces the record at path with s.
func WriteFuzzSession(path string, s FuzzSession) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".fuzz-session-*")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}

// ReadFuzzSession reads the record at path; ok is false when there is none.
func ReadFuzzSession(path string) (s FuzzSession, ok bool, err error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: the cache's own record
	if errors.Is(err, os.ErrNotExist) {
		return FuzzSession{}, false, nil
	}
	if err != nil {
		return FuzzSession{}, false, err
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return FuzzSession{}, false, fmt.Errorf("read fuzz session %s: %w", path, err)
	}
	return s, true, nil
}

// RecordFuzzSession writes the crashers res found to the record at path, as
// the latest session's. res's outcomes line up with targets.
func RecordFuzzSession(path string, targets []FuzzTarget, res FuzzRunResult) error {
	var s FuzzSession
	for i, o := range res.Outcomes {
		if i >= len(targets) {
			break
		}
		for _, name := range o.NewCrashers {
			c, err := NewSessionCrasher(targets[i].Dir, targets[i].Func, name)
			if err != nil {
				return err
			}
			s.Crashers = append(s.Crashers, c)
		}
	}
	return WriteFuzzSession(path, s)
}
