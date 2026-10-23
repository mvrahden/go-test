package testkit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// A store holds CLI builds for reuse, as files named by what is in them:
//
//   - gotest-cli-build-<key>: a published build. It appears by a rename of
//     a complete file and is never written again; every user copies it into
//     a dir of its own and runs the copy.
//   - gotest-cli-use-<key>-<pid>: pid's last use of that build.
//   - gotest-cli-hold-<key>: a dir holding a stamp named <pid>-<n> for the
//     process building key, so others wait for that build instead of making
//     their own. It appears by a rename of a dir already holding the stamp.
//   - gotest-cli-mark-<n>: such a dir being made.
//   - gotest-cli-<n>: a dir that a build or a private copy sits in, stamped
//     with its owner's pid in .pid.
//
// No lock guards any of them. A sweep removes a build only once nobody used
// it for a while; one that a user is copying at that moment either stays
// readable to the copy or is gone before it opens, and then the user builds.
const (
	cliDirPrefix   = "gotest-cli-"
	buildPrefix    = cliDirPrefix + "build-"
	usePrefix      = cliDirPrefix + "use-"
	buildingPrefix = cliDirPrefix + "hold-"
	markPrefix     = cliDirPrefix + "mark-"
)

// keyedLinger is how long a published build outlives its last use, so the
// next process of the same run reuses it instead of building again.
const keyedLinger = 10 * time.Minute

// cliDirGrace is how long a CLI dir with no pid file is left alone: its
// creator stamps it right after creating it, and an earlier testkit, which
// stamped none, may still be running from it. Files the sweep does not know
// get the same grace.
const cliDirGrace = 24 * time.Hour

// buildWait is how long a build marker holds others off; past it the
// builder is taken to be hung, and another process builds.
const buildWait = 10 * time.Minute

// pollEvery is how often a process waiting for another's build looks again.
const pollEvery = 25 * time.Millisecond

// pidFile is the owner's stamp in a CLI dir.
const pidFile = ".pid"

// NewCLIDir creates a temp dir for a CLI build, stamped with this process's
// pid, after sweeping the CLI dirs no live process owns.
func NewCLIDir() (string, error) {
	tempStore().sweep()
	return newStampedDir(os.TempDir(), os.Getpid())
}

// newStampedDir creates a CLI dir in parent, stamped with pid.
func newStampedDir(parent string, pid int) (string, error) {
	dir, err := os.MkdirTemp(parent, cliDirPrefix)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, pidFile), []byte(strconv.Itoa(pid)), 0o600); err != nil {
		_ = os.RemoveAll(dir)
		return "", err
	}
	return dir, nil
}

// keyOf and buildOf are how a reuse keys and builds a CLI; tests swap them.
// beforeMark runs before each try to create a build marker, so a test can
// contend for it; beforeTake just before a dead marker is removed;
// renameMarker moves a staged marker into place.
var (
	keyOf        = cliKey
	buildOf      = buildCLI
	beforeMark   = func(marker string) {}
	beforeTake   = func(marker string) {}
	renameMarker = os.Rename
)

// resolveCLI returns a private copy of a CLI built from root's sources:
// copied from the build an earlier process published when one is there,
// else built and published for the next process.
func resolveCLI(ctx context.Context, root string) (string, error) {
	private, err := NewCLIDir()
	if err != nil {
		return "", err
	}
	return reuse(ctx, tempStore(), root, private)
}

// reuse puts a CLI built from root's sources into private, by way of s.
// Sources it cannot key are built straight into private.
func reuse(ctx context.Context, s store, root, private string) (string, error) {
	key, err := keyOf(ctx, root)
	if err != nil {
		return buildOf(ctx, root, private)
	}
	return s.get(ctx, key, private, func(ctx context.Context, dir string) (string, error) {
		return buildOf(ctx, root, dir)
	})
}

func tempStore() store {
	return store{dir: os.TempDir(), pid: os.Getpid(), rules: defaultSweep}
}

// store is a dir of CLI builds, used as process pid.
type store struct {
	dir   string
	pid   int
	rules sweepRules
}

// get copies the build published under key into private and returns the
// copy. With none published, it waits for another process's build of key,
// or builds and publishes one itself.
func (s store) get(ctx context.Context, key, private string, build func(context.Context, string) (string, error)) (string, error) {
	for {
		if binary, ok := s.claim(key, private); ok {
			return binary, nil
		}
		release, busy, err := s.markBuild(ctx, key)
		if err != nil {
			return "", err
		}
		if !busy {
			defer release()
			// Published while this process marked it.
			if binary, ok := s.claim(key, private); ok {
				return binary, nil
			}
			return s.publish(ctx, key, private, build)
		}
		if err := s.waitForBuild(ctx, key); err != nil {
			return "", err
		}
	}
}

// publish builds key in a dir of its own, copies the binary into private,
// and moves it to its published name in one rename, so no reader sees part
// of it.
func (s store) publish(ctx context.Context, key, private string, build func(context.Context, string) (string, error)) (string, error) {
	stage, err := newStampedDir(s.dir, s.pid)
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)
	built, err := build(ctx, stage)
	if err != nil {
		return "", err
	}
	binary, err := copyExecutable(built, private)
	if err != nil {
		return "", err
	}
	published := filepath.Join(s.dir, buildPrefix+key)
	for range 5 {
		if exists(published) || os.Rename(built, published) == nil {
			break
		}
		// A scanner may hold a fresh executable open for a moment.
		time.Sleep(20 * time.Millisecond)
	}
	return binary, nil
}

// claim records this process's use of key's build and copies it into
// private. It fails when no build is published.
func (s store) claim(key, private string) (string, bool) {
	published := filepath.Join(s.dir, buildPrefix+key)
	if !exists(published) {
		return "", false
	}
	pid := strconv.Itoa(s.pid)
	if os.WriteFile(filepath.Join(s.dir, usePrefix+key+"-"+pid), []byte(pid), 0o600) != nil {
		return "", false
	}
	binary, err := copyExecutable(published, private)
	return binary, err == nil
}

// markBuild creates key's build marker and returns its release, or busy
// while another live process builds key. A marker no longer live is taken
// over; it tries until one of the two holds, ctx ends, or creating the
// marker fails a few times over for another reason.
func (s store) markBuild(ctx context.Context, key string) (release func(), busy bool, err error) {
	marker := filepath.Join(s.dir, buildingPrefix+key)
	failed := 0
	for {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		beforeMark(marker)
		stamp, err := s.createMarker(marker)
		if err == nil {
			return func() { unmarkBuild(marker, stamp) }, false, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			// A scanner may hold the fresh stamp open for a moment.
			if failed++; failed >= 5 {
				return nil, false, err
			}
			select {
			case <-ctx.Done():
			case <-time.After(20 * time.Millisecond):
			}
			continue
		}
		if s.rules.markerLive(marker) {
			return nil, true, nil
		}
		// The builder died, hung or is releasing; take its place.
		s.rules.removeDeadMarker(marker)
		if exists(marker) {
			// Refused for now, or another builder's since; look again shortly.
			select {
			case <-ctx.Done():
			case <-time.After(pollEvery):
			}
		}
	}
}

// createMarker creates the build marker with this process's stamp already
// in it, and returns the stamp. The rename fails while any marker is there,
// even an empty one being released.
func (s store) createMarker(marker string) (string, error) {
	staged, err := os.MkdirTemp(s.dir, markPrefix)
	if err != nil {
		return "", err
	}
	f, err := os.CreateTemp(staged, strconv.Itoa(s.pid)+"-")
	if err == nil {
		err = f.Close()
	}
	if err == nil {
		if err = renameMarker(staged, marker); err == nil {
			return filepath.Join(marker, filepath.Base(f.Name())), nil
		}
		if exists(marker) {
			err = fs.ErrExist
		}
	}
	_ = os.RemoveAll(staged)
	return "", err
}

// unmarkBuild removes a build marker's stamp, then the marker once empty.
// The marker stays while another builder's stamp is in it, or a reader holds
// it open; empty, it holds nobody off.
func unmarkBuild(marker, stamp string) {
	for range 10 {
		if err := os.Remove(stamp); err == nil || errors.Is(err, fs.ErrNotExist) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	_ = os.Remove(marker)
}

// waitForBuild returns once key is published, or its build marker is gone
// or no longer live.
func (s store) waitForBuild(ctx context.Context, key string) error {
	marker := filepath.Join(s.dir, buildingPrefix+key)
	published := filepath.Join(s.dir, buildPrefix+key)
	for !exists(published) && s.rules.markerLive(marker) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollEvery):
		}
	}
	return nil
}

// copyExecutable copies the file at from into dir as the CLI binary.
func copyExecutable(from, dir string) (string, error) {
	data, err := os.ReadFile(from) //nolint:gosec // G304: a build in a CLI store
	if err != nil {
		return "", err
	}
	private := filepath.Join(dir, cliName())
	// A concurrent fork must not inherit a write handle to an executable
	// another goroutine is about to start (the same rule as runner.copyFile).
	syscall.ForkLock.RLock()
	defer syscall.ForkLock.RUnlock()
	if err := os.WriteFile(private, data, 0o700); err != nil { //nolint:gosec // G306: the caller must execute its private CLI
		return "", err
	}
	return private, nil
}

// cliKey names a build of root's ./cmd/gotest by what goes into it: the
// compile's build ID, which hashes every source and embedded file of the
// package and its imports, the go settings the link reads, and root itself,
// so another tree, or the same tree with other sources, keys differently.
func cliKey(ctx context.Context, root string) (string, error) {
	h := sha256.New()
	h.Write([]byte(root + "\n"))
	for _, args := range [][]string{
		{"list", "-export", "-f", "{{.BuildID}}", "./cmd/gotest"},
		{"env", "GOFLAGS", "CGO_ENABLED", "CGO_LDFLAGS", "GOVERSION"},
	} {
		cmd := exec.CommandContext(ctx, "go", args...) //nolint:gosec // G204: go tool with controlled arguments
		cmd.Dir = root
		out, err := cmd.Output()
		if err != nil {
			return "", err
		}
		h.Write(out)
	}
	return hex.EncodeToString(h.Sum(nil))[:20], nil
}

// sweepRules decides what a sweep takes.
type sweepRules struct {
	alive  func(pid int) bool
	linger time.Duration // a published build's, after its last use
	grace  time.Duration // an unstamped dir's, or an unknown file's
	wait   time.Duration // a build marker's
}

var defaultSweep = sweepRules{alive: processAlive, linger: keyedLinger, grace: cliDirGrace, wait: buildWait}

// markerLive reports whether the build marker holds others off: it holds
// the stamp of a live pid, younger than the wait.
func (r sweepRules) markerLive(marker string) bool {
	live, _ := r.judgeMarker(marker)
	return live
}

// judgeMarker is markerLive, and the stamps that hold nobody off.
func (r sweepRules) judgeMarker(marker string) (live bool, dead []string) {
	entries, err := os.ReadDir(marker)
	if err != nil {
		return false, nil
	}
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue // removed since
		}
		pid, err := strconv.Atoi(strings.SplitN(e.Name(), "-", 2)[0])
		if err == nil && r.alive(pid) && time.Since(info.ModTime()) < r.wait {
			return true, nil
		}
		dead = append(dead, filepath.Join(marker, e.Name()))
	}
	return false, dead
}

// removeDeadMarker removes the marker when it holds nobody off: each stamp
// by its own name, never given another, and then the marker only if empty.
// A marker put in its place since holds a stamp, so it stays.
func (r sweepRules) removeDeadMarker(marker string) {
	live, dead := r.judgeMarker(marker)
	if live {
		return
	}
	beforeTake(marker)
	for _, stamp := range dead {
		_ = os.Remove(stamp)
	}
	_ = os.Remove(marker)
}

// sweep removes what in s no live process needs any more: a dir stamped
// only by dead pids, or unstamped and older than the grace; a build and its
// use records once none is younger than the linger; a build marker no
// longer live; anything else older than the grace. A removal that fails is
// left for a later sweep.
func (s store) sweep() {
	paths, err := filepath.Glob(filepath.Join(s.dir, cliDirPrefix+"*"))
	if err != nil {
		return
	}
	for _, path := range paths {
		s.sweepOne(path)
	}
}

func (s store) sweepOne(path string) {
	info, err := os.Lstat(path)
	if err != nil {
		return
	}
	name, age := info.Name(), time.Since(info.ModTime())
	switch {
	case strings.HasPrefix(name, buildingPrefix):
		s.rules.removeDeadMarker(path)
	case info.IsDir():
		// Read once: a dir stamped after a first look is not dead. A stamp
		// being written holds no pid yet, so it counts as none.
		pid, stamped := readPID(filepath.Join(path, pidFile))
		if stamped && !s.rules.alive(pid) || !stamped && age >= s.rules.grace {
			_ = os.RemoveAll(path)
		}
	case strings.HasPrefix(name, buildPrefix):
		if s.lastUse(strings.TrimPrefix(name, buildPrefix), info) >= s.rules.linger {
			_ = os.Remove(path)
		}
	case strings.HasPrefix(name, usePrefix):
		if age >= s.rules.linger {
			_ = os.Remove(path)
		}
	case age >= s.rules.grace:
		_ = os.Remove(path)
	}
}

// lastUse is how long ago key's build, published as info, was published
// or last used.
func (s store) lastUse(key string, info fs.FileInfo) time.Duration {
	latest := info.ModTime()
	uses, _ := filepath.Glob(filepath.Join(s.dir, usePrefix+key+"-*"))
	for _, use := range uses {
		if u, err := os.Stat(use); err == nil && u.ModTime().After(latest) {
			latest = u.ModTime()
		}
	}
	return time.Since(latest)
}

// cliDirAlive reports whether a live process stamped dir.
func cliDirAlive(dir string) bool {
	pid, ok := readPID(filepath.Join(dir, pidFile))
	return ok && processAlive(pid)
}

// readPID reads the pid a CLI dir's stamp holds.
func readPID(path string) (int, bool) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: a pid file in a CLI store
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	return pid, err == nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
