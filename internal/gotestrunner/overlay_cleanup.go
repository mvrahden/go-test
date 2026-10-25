package gotestrunner

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/proctree"
)

// CleanStaleOverlays removes the temporary overlay and work directories whose
// owning process is no longer alive, and evicts aged-out cache entries.
//
// A dir names its owner by pid, by the boot it ran in (on Linux the boot id,
// elsewhere the host name) and, on Linux, by its pid namespace. Only a dir
// stamped in this boot and namespace whose pid runs no process is removed:
// a pid read elsewhere, such as on another host sharing TMP, names some other
// process or none. A reused pid reads as alive, so its dir waits for it.
func CleanStaleOverlays() {
	cleanStaleTmpOverlays()
	cleanOldCacheEntries()
}

// keepFile marks a dir the sweep leaves alone for keptMaxAge: a --debug
// run's, kept for inspection.
const keepFile = ".keep"

const keptMaxAge = 7 * 24 * time.Hour

// workDirGrace is how long a work dir with no readable stamp is left alone:
// an earlier gotest, which wrote none, may still be running in it.
const workDirGrace = keptMaxAge

// overlayDirGrace is how long an overlay dir with no readable stamp is left
// alone: its writer creates it first and writes the stamp right after.
const overlayDirGrace = time.Minute

// unknownID is the boot or pid namespace of a process that cannot read its
// own. It matches none, itself included.
const unknownID = "unknown"

func cleanStaleTmpOverlays() {
	for glob, grace := range map[string]time.Duration{"gotest-overlay-*": overlayDirGrace, "gotest-work-*": workDirGrace} {
		dirs, err := filepath.Glob(filepath.Join(os.TempDir(), glob))
		if err != nil {
			continue
		}
		for _, dir := range dirs {
			info, err := os.Stat(dir)
			if err != nil || !info.IsDir() || exists(filepath.Join(dir, keepFile)) && time.Since(info.ModTime()) < keptMaxAge {
				continue
			}
			switch judgeOwner(dir) {
			case ownerLive, ownerElsewhere:
				continue
			case ownerUnknown:
				if time.Since(info.ModTime()) < grace {
					continue
				}
			case ownerGone:
			}
			os.RemoveAll(dir)
		}
	}
}

// ownerState is what a sweep can tell of a dir's owner.
type ownerState int

const (
	ownerUnknown   ownerState = iota // no stamp, or none written whole yet
	ownerElsewhere                   // stamped in another or an unknown boot or pid namespace
	ownerLive
	ownerGone
)

// Variables so tests can stand in for another boot or namespace.
var (
	selfPIDNamespace = pidNamespace
	selfBoot         = bootID
)

// judgeOwner reads dir's stamp and tells whether its owner still runs.
func judgeOwner(dir string) ownerState {
	pid, ok := ownerPID(dir)
	if !ok {
		return ownerUnknown
	}
	if boot, ns, stamped := ownerIdentity(dir); stamped && (!sameID(boot, selfBoot()) || !sameID(ns, selfPIDNamespace())) {
		return ownerElsewhere
	}
	if proctree.Alive(pid) {
		return ownerLive
	}
	return ownerGone
}

// sameID tells whether a stamped boot or namespace is this process's own.
// One that cannot be read matches none; a platform without pid namespaces
// stamps none and reads none.
func sameID(stamped, self string) bool {
	return stamped == self && stamped != unknownID
}

// ownerPID reads the pid a current gotest writes into dir; ok is false when
// there is none, or none written whole yet.
func ownerPID(dir string) (pid int, ok bool) {
	data, err := os.ReadFile(filepath.Join(dir, pidFile))
	if err != nil {
		return 0, false
	}
	pid, err = strconv.Atoi(strings.TrimSpace(string(data)))
	return pid, err == nil
}

// pidFile holds the owner's pid alone, as earlier gotest versions read it;
// ownerFile beside it holds the rest of its identity, as "key value" lines.
const (
	pidFile   = ".pid"
	ownerFile = ".owner"
)

// ownerIdentity reads the boot and pid namespace dir's owner wrote; stamped
// is false for a writer that wrote no identity file. A file that is there but
// cannot be read names an owner that cannot be told, so it matches none.
func ownerIdentity(dir string) (boot, ns string, stamped bool) {
	data, err := os.ReadFile(filepath.Join(dir, ownerFile))
	if errors.Is(err, fs.ErrNotExist) {
		return "", "", false
	}
	if err != nil {
		return unknownID, unknownID, true
	}
	for line := range strings.Lines(string(data)) {
		key, value, _ := strings.Cut(strings.TrimSpace(line), " ")
		switch key {
		case "boot":
			boot = value
		case "pidns":
			ns = value
		}
	}
	return boot, ns, true
}

// writePIDFile stamps dir with this run's identity, then its pid through a
// renamed temp file, so a sweep that reads the pid finds both whole.
func writePIDFile(dir string) error {
	id := "boot " + selfBoot() + "\n"
	if ns := selfPIDNamespace(); ns != "" {
		id += "pidns " + ns + "\n"
	}
	if err := os.WriteFile(filepath.Join(dir, ownerFile), []byte(id), 0o600); err != nil {
		return err
	}
	tmp := filepath.Join(dir, pidFile+".tmp")
	if err := os.WriteFile(tmp, []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, pidFile))
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

const cacheMaxAge = 7 * 24 * time.Hour

// cleanOldCacheEntries evicts test-binary cache entries, and the overlay
// entries older CLIs pinned in other modules still write, that no run has
// touched within cacheMaxAge.
func cleanOldCacheEntries() {
	root, err := cacheRoot()
	if err != nil {
		return
	}
	sweepBinaryCache(root)
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
