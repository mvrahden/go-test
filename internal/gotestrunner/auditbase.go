package gotestrunner

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// gotest audit --base exports a commit into a directory of the gotest cache
// fixed per checkout: Go keys a package's build by its directory, so a tree
// at a new path on every audit would add every package to the build cache
// anew. A lock file beside the directory admits one audit of a checkout at
// a time.

// AuditBase is the export directory of one checkout, held under its lock.
type AuditBase struct {
	Dir     string
	release func()
}

// Release lets the next audit of the checkout have the directory.
func (b *AuditBase) Release() { b.release() }

// auditBaseRoot is where the exports live.
func auditBaseRoot() (string, error) { return auditRoot("audit") }

// auditMutateRoot is where audit --mutate keeps each checkout's mutants:
// a fixed overlay path keeps one binary cache entry per checkout.
func auditMutateRoot() (string, error) { return auditRoot("audit-mutate") }

func auditRoot(sub string) (string, error) {
	root, err := cacheRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, sub), nil
}

// auditBaseKey names the checkout at top among the exports.
func auditBaseKey(top string) string {
	sum := sha256.Sum256([]byte(top))
	return hex.EncodeToString(sum[:])[:16]
}

// LockAuditBase takes the export directory of the checkout at top, waiting
// while another audit of it holds the directory.
func LockAuditBase(top string) (*AuditBase, error) {
	root, err := auditBaseRoot()
	if err != nil {
		return nil, err
	}
	return lockAuditDir(root, top)
}

// LockAuditMutate takes the mutants' directory of the checkout at top, as
// LockAuditBase takes its export's.
func LockAuditMutate(top string) (*AuditBase, error) {
	root, err := auditMutateRoot()
	if err != nil {
		return nil, err
	}
	return lockAuditDir(root, top)
}

func lockAuditDir(root, top string) (*AuditBase, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	key := auditBaseKey(top)
	for {
		unlock, err := lockFile(filepath.Join(root, key+".lock"))
		if errors.Is(err, errLockReplaced) {
			continue
		}
		if err != nil {
			return nil, err
		}
		return &AuditBase{Dir: filepath.Join(root, key), release: unlock}, nil
	}
}

// ClearAuditBases removes every export no audit holds, with its lock. It
// returns how many it removed, how many it left to an audit using them,
// and the directory they live in.
func ClearAuditBases() (removed, inUse int, dir string, err error) {
	root, err := auditBaseRoot()
	if err != nil {
		return 0, 0, "", err
	}
	removed, inUse = clearAuditBases(root)
	return removed, inUse, root, nil
}

// ClearAuditMutations removes every mutants' directory no audit holds, as
// ClearAuditBases does the exports.
func ClearAuditMutations() (removed, inUse int, dir string, err error) {
	root, err := auditMutateRoot()
	if err != nil {
		return 0, 0, "", err
	}
	removed, inUse = clearAuditBases(root)
	return removed, inUse, root, nil
}

func clearAuditBases(root string) (removed, inUse int) {
	entries, _ := os.ReadDir(root)
	keys := map[string]bool{}
	for _, e := range entries {
		keys[strings.TrimSuffix(e.Name(), ".lock")] = true
	}
	for key := range keys {
		lock := filepath.Join(root, key+".lock")
		unlock, err := tryLockFile(lock)
		if err != nil {
			inUse++
			continue
		}
		if _, err := os.Stat(filepath.Join(root, key)); err == nil {
			_ = os.RemoveAll(filepath.Join(root, key))
			removed++
		}
		releaseEvicted(lock, unlock)
	}
	return removed, inUse
}
