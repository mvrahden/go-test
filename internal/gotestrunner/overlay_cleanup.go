package gotestrunner

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/proctree"
)

// CleanStaleOverlays removes the temporary overlay and work directories and
// the cache staging directories whose owning process is no longer alive, and
// evicts aged-out cache entries.
func CleanStaleOverlays() {
	cleanStaleTmpOverlays()
	cleanStaleStaging()
	cleanOldCacheEntries()
}

// workDirGrace is how long a work dir with no pid file is left alone: an
// earlier gotest, which wrote none, may still be running in it.
const workDirGrace = 24 * time.Hour

// overlayDirGrace is how long an overlay dir with no pid file is left alone:
// its writer creates it first and writes the pid right after.
const overlayDirGrace = time.Minute

func cleanStaleTmpOverlays() {
	for glob, grace := range map[string]time.Duration{"gotest-overlay-*": overlayDirGrace, "gotest-work-*": workDirGrace} {
		dirs, err := filepath.Glob(filepath.Join(os.TempDir(), glob))
		if err != nil {
			continue
		}
		for _, dir := range dirs {
			info, err := os.Stat(dir)
			if err != nil || !info.IsDir() {
				continue
			}
			if isOverlayAlive(dir) {
				continue
			}
			if !hasPID(dir) && time.Since(info.ModTime()) < grace {
				continue
			}
			os.RemoveAll(dir)
		}
	}
}

// hasPID reports whether dir carries the pid file a current gotest writes.
func hasPID(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".pid"))
	return err == nil
}

// cleanStaleStaging removes cache entries still being written by a run that
// is gone; the writer's pid is in the directory's name.
func cleanStaleStaging() {
	root, err := cacheRoot()
	if err != nil {
		return
	}
	dirs, err := filepath.Glob(filepath.Join(root, "overlays", "*.staging-*"))
	if err != nil {
		return
	}
	for _, dir := range dirs {
		pid, err := strconv.Atoi(dir[strings.LastIndex(dir, "-")+1:])
		if err != nil || proctree.Alive(pid) {
			continue
		}
		os.RemoveAll(dir)
	}
}

const cacheMaxAge = 7 * 24 * time.Hour

// cleanOldCacheEntries evicts overlay and test-binary cache entries that no
// run has touched within cacheMaxAge.
func cleanOldCacheEntries() {
	root, err := cacheRoot()
	if err != nil {
		return
	}
	now := time.Now()
	for _, sub := range []string{"overlays", "bin"} {
		dir := filepath.Join(root, sub)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			if now.Sub(info.ModTime()) > cacheMaxAge {
				os.RemoveAll(filepath.Join(dir, e.Name()))
			}
		}
	}
}

func isOverlayAlive(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, ".pid"))
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return false
	}
	return proctree.Alive(pid)
}
