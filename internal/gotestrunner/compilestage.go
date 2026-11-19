package gotestrunner

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// stagedBinary is one package's slot in a group's stage dir.
type stagedBinary struct {
	Package string
	Path    string      // <stage>/<testBinaryName>
	Cached  string      // the cache entry, "" without a cache
	placed  os.FileInfo // the file put at Path before the build, nil for none
	linked  bool        // placed by a hard link
	damaged bool        // the entry no longer matched its record
}

// stageGroup creates stageDir and places each package's cached binary in
// it under go's name, so go confirms it or replaces it. An entry that
// cannot be locked, or no longer matches its record, is not placed: go
// links that package again.
func stageGroup(group []string, stageDir, cacheDir string) ([]stagedBinary, error) {
	if err := os.MkdirAll(stageDir, 0o755); err != nil {
		return nil, err
	}
	staged := make([]stagedBinary, len(group))
	for i, pkg := range group {
		b := &staged[i]
		b.Package = pkg
		b.Path = filepath.Join(stageDir, testBinaryName(pkg, runtime.GOOS))
		if cacheDir != "" {
			b.Cached = filepath.Join(cacheDir, sanitizePkgName(pkg)+".test")
			place(b)
		}
	}
	return staged, nil
}

// place puts b's cache entry at its slot, under the entry's lock.
func place(b *stagedBinary) {
	if _, err := os.Stat(b.Cached); err != nil {
		return
	}
	unlock, err := lockFile(b.Cached + ".lock")
	if err != nil {
		return // errLockReplaced included: the entry was evicted
	}
	defer unlock()
	record, recorded := readSum(b.Cached)
	if !recorded {
		return
	}
	if sum, err := sumFile(b.Cached); err != nil || sum != record {
		b.damaged = err == nil
		return
	}
	linked, err := linkOrCopy(b.Cached, b.Path)
	if err != nil {
		_ = os.Remove(b.Path)
		return
	}
	if info, err := os.Stat(b.Path); err == nil {
		b.placed, b.linked = info, linked
	}
}

type readiness int

const (
	notReady  readiness = iota
	confirmed           // the placed file, touched by go: up to date
	rebuilt             // a file go wrote
)

// binaryReady judges one slot. exited is go's exit: -1 while it runs. A
// placed file go has not touched is ready only once go exited 0; a new
// file only once go exited, unless go renames it into place.
func binaryReady(placed, now os.FileInfo, renameSafe bool, exited int) readiness {
	switch {
	case now == nil:
		return notReady
	case placed != nil && os.SameFile(placed, now):
		if now.ModTime().After(placed.ModTime()) || exited == 0 {
			return confirmed
		}
		return notReady
	case renameSafe || exited >= 0:
		return rebuilt
	default:
		return notReady
	}
}

// renameSafe reports whether go renames a linked binary into stageDir
// rather than writing it in place: not on Windows, not into a setgid dir,
// and not from a work dir under GOCACHE, which cmd/go copies from.
func renameSafe(goos string, stageDir os.FileInfo, goTmp, goCache string) bool {
	if goos == "windows" || goCache == "" || strings.HasPrefix(goTmp, goCache) {
		return false
	}
	return stageDir == nil || stageDir.Mode()&fs.ModeSetgid == 0
}

// publish makes a rebuilt binary the cache entry, by a rename: a run
// executing the old entry keeps it. A confirmed copy marks its entry used,
// since go's touch reached only the copy. The cache is an optimization:
// whatever fails here is dropped.
func publish(b *stagedBinary, r readiness) {
	if b.Cached == "" {
		return
	}
	switch {
	case r == rebuilt:
		unlock, err := lockFile(b.Cached + ".lock")
		if err != nil {
			return
		}
		defer unlock()
		sum, err := sumFile(b.Path)
		if err != nil {
			return
		}
		tmp := b.Cached + ".tmp-" + strconv.Itoa(os.Getpid())
		if _, err := linkOrCopy(b.Path, tmp); err != nil {
			_ = os.Remove(tmp)
			return
		}
		if err := os.Rename(tmp, b.Cached); err != nil {
			_ = os.Remove(tmp)
			return
		}
		_ = writeSum(b.Cached, sum)
		if b.damaged {
			fmt.Fprintf(os.Stderr, "gotest: the cached test binary of %s did not match its record and was linked again\n", b.Package)
		}
	case r == confirmed && b.placed != nil && !b.linked:
		now := time.Now()
		_ = os.Chtimes(b.Cached, now, now)
	}
}
