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
	"sync/atomic"
	"time"

	"github.com/mvrahden/go-test/internal/proctree"
)

// cliDirPrefix starts the name of every temp dir a CLI is built into.
const cliDirPrefix = "gotest-cli-"

// keyedPrefix starts the name of a CLI dir published for reuse under its
// sources' key.
const keyedPrefix = cliDirPrefix + "key-"

// gonePrefix starts the name a swept dir is moved to before it is removed.
const gonePrefix = cliDirPrefix + "gone-"

// keyedLinger is how long a published build outlives its last user, so the
// next process of the same run reuses it instead of building again.
const keyedLinger = 10 * time.Minute

// cliDirGrace is how long a CLI dir with no pid file is left alone: its
// creator stamps it right after creating it, and an earlier testkit, which
// stamped none, may still be running from it.
const cliDirGrace = 24 * time.Hour

// lockSuffix ends the name of a published build's lock, which sits beside
// its dir: Windows refuses to move a dir while a file in it is open. A claim
// holds it while it checks the binary and stamps the dir, a sweep while it
// judges and moves the dir aside, a publisher while it moves a build in, so a
// sweep never takes a build a process has claimed or is claiming.
const lockSuffix = ".lock"

// pidFile is the creator's stamp; a process reusing the dir adds its own
// pidFile.<pid> beside it.
const pidFile = ".pid"

// NewCLIDir creates a temp dir for a CLI build, stamped with this process's
// pid, after sweeping the CLI dirs no live process owns.
func NewCLIDir() (string, error) {
	sweepCLIDirs()
	return newStampedDir(os.Getpid())
}

// newStampedDir creates a CLI dir in the temp dir, stamped with pid.
func newStampedDir(pid int) (string, error) {
	dir, err := os.MkdirTemp("", cliDirPrefix)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, pidFile), []byte(strconv.Itoa(pid)), 0o600); err != nil {
		_ = os.RemoveAll(dir)
		return "", err
	}
	return dir, nil
}

// resolveCLI returns a CLI built from root's sources: the one an earlier
// process built from the same sources when it is still there, else a fresh
// build, published under the sources' key for the next process.
func resolveCLI(ctx context.Context, root string) (string, error) {
	key, keyErr := cliKey(ctx, root)
	keyed := filepath.Join(os.TempDir(), keyedPrefix+key)
	if keyErr == nil {
		if binary, ok := claimCLI(keyed); ok {
			sweepCLIDirs()
			return binary, nil
		}
	}
	dir, err := NewCLIDir()
	if err != nil {
		return "", err
	}
	binary, err := BuildCLI(ctx, root, dir)
	if err != nil {
		_ = os.RemoveAll(dir)
		return "", err
	}
	if keyErr != nil {
		return binary, nil
	}
	if publish(dir, keyed) {
		return filepath.Join(keyed, cliName()), nil
	}
	// Another process published the same build first.
	if published, ok := claimCLI(keyed); ok {
		_ = os.RemoveAll(dir)
		return published, nil
	}
	return binary, nil
}

// claimCLI stamps the keyed dir with this process's pid, so no sweep takes
// it while the process runs, and returns its binary when it has one.
func claimCLI(keyed string) (string, bool) { return claimAs(keyed, os.Getpid(), proctree.Alive) }

// claimAs is claimCLI for pid, judging other stamps by alive.
func claimAs(keyed string, pid int, alive func(int) bool) (string, bool) {
	unlock, ok := lockDir(keyed, false)
	if !ok {
		return "", false
	}
	defer unlock()
	binary := filepath.Join(keyed, cliName())
	if _, err := os.Stat(binary); err != nil {
		return "", false
	}
	stamp := filepath.Join(keyed, pidFile+"."+strconv.Itoa(pid))
	if os.WriteFile(stamp, []byte(strconv.Itoa(pid)), 0o600) != nil {
		return "", false
	}
	pruneDeadStamps(keyed, alive)
	return binary, true
}

// publish moves the build in dir to keyed, under keyed's lock.
func publish(dir, keyed string) bool {
	unlock, ok := lockDir(keyed, false)
	if !ok {
		return false
	}
	defer unlock()
	return os.Rename(dir, keyed) == nil
}

// lockDir takes the lock beside dir, creating it, waiting for it unless try.
// It fails when try finds it held. Unlocking removes the lock when dir is
// gone, so no lock outlives its dir; a waiter whose lock was removed
// meanwhile opens the current one, where try fails.
func lockDir(dir string, try bool) (unlock func(), ok bool) {
	path := dir + lockSuffix
	for range 3 {
		f, err := openLock(path)
		if err != nil {
			return nil, false
		}
		if lockDirFD(f, try) != nil {
			_ = f.Close()
			return nil, false
		}
		held, err := f.Stat()
		current, err2 := os.Stat(path)
		if err == nil && err2 == nil && os.SameFile(held, current) {
			return func() {
				if _, err := os.Lstat(dir); errors.Is(err, fs.ErrNotExist) {
					_ = os.Remove(path)
				}
				unlockFD(f)
				_ = f.Close()
			}, true
		}
		unlockFD(f)
		_ = f.Close()
		if try {
			break
		}
	}
	return nil, false
}

// lockDirFD takes the lock lockDir opened; tests swap it.
var lockDirFD = lockFD

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

// sweepRules decides which CLI dirs a sweep takes.
type sweepRules struct {
	alive  func(pid int) bool
	linger time.Duration // a published build's, after its last use
	grace  time.Duration // an unstamped dir's
}

var defaultSweep = sweepRules{alive: proctree.Alive, linger: keyedLinger, grace: cliDirGrace}

// sweepCLIDirs removes the CLI dirs in the temp dir that no live process
// owns: a published build once no user touched it for keyedLinger; any other
// dir stamped only by dead pids at once, and an unstamped one once it is
// older than cliDirGrace.
func sweepCLIDirs() {
	dirs, err := filepath.Glob(filepath.Join(os.TempDir(), cliDirPrefix+"*"))
	if err != nil {
		return
	}
	for _, dir := range dirs {
		sweepCLIDir(dir, defaultSweep)
	}
}

func sweepCLIDir(dir string, rules sweepRules) {
	if strings.HasPrefix(filepath.Base(dir), gonePrefix) {
		// Swept already; what is left is what Windows kept open.
		_ = os.RemoveAll(dir)
		return
	}
	if lockOf, isLock := strings.CutSuffix(dir, lockSuffix); isLock {
		// A lock left by a process that died holding it; unlocking removes
		// it once its dir is gone.
		if _, err := os.Lstat(lockOf); errors.Is(err, fs.ErrNotExist) {
			if unlock, ok := lockDir(lockOf, true); ok {
				unlock()
			}
		}
		return
	}
	if !stale(dir, rules) {
		return
	}
	if !strings.HasPrefix(filepath.Base(dir), keyedPrefix) {
		// No claim takes an unpublished dir: an earlier testkit's, or a
		// build's whose owner is gone.
		_ = os.RemoveAll(dir)
		return
	}
	unlock, ok := lockDir(dir, true)
	if !ok {
		return // being claimed or swept
	}
	// Judged again under the lock: a claim may have stamped it meanwhile.
	if !stale(dir, rules) {
		unlock()
		return
	}
	// Moved aside before the lock is released: a claim that waited for it
	// then finds no binary, and a build published meanwhile is never the
	// one removed.
	aside := filepath.Join(filepath.Dir(dir), gonePrefix+strconv.Itoa(os.Getpid())+"-"+strconv.FormatInt(goneSeq.Add(1), 10))
	err := renameDir(dir, aside)
	unlock()
	if err == nil {
		_ = os.RemoveAll(aside)
	}
}

var goneSeq atomic.Int64

// stale reports whether rules let a sweep take dir. A published build that
// lost its binary is never reused, so it goes at once.
func stale(dir string, rules sweepRules) bool {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() || dirAlive(dir, rules.alive) {
		return false
	}
	age := time.Since(info.ModTime())
	if strings.HasPrefix(filepath.Base(dir), keyedPrefix) {
		_, err := os.Stat(filepath.Join(dir, cliName()))
		return err != nil || age >= rules.linger
	}
	return len(stampsOf(dir)) > 0 || age >= rules.grace
}

// cliDirAlive reports whether a live process stamped dir.
func cliDirAlive(dir string) bool { return dirAlive(dir, proctree.Alive) }

func dirAlive(dir string, alive func(int) bool) bool {
	for _, stamp := range stampsOf(dir) {
		if stampAlive(stamp, alive) {
			return true
		}
	}
	return false
}

// pruneDeadStamps drops the stamps of reusers that are gone, so a dir many
// processes reuse does not collect them.
func pruneDeadStamps(dir string, alive func(int) bool) {
	for _, stamp := range stampsOf(dir) {
		if filepath.Base(stamp) != pidFile && !stampAlive(stamp, alive) {
			_ = os.Remove(stamp)
		}
	}
}

// stampsOf lists dir's pid files.
func stampsOf(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var stamps []string
	for _, e := range entries {
		if name := e.Name(); name == pidFile || strings.HasPrefix(name, pidFile+".") {
			stamps = append(stamps, filepath.Join(dir, name))
		}
	}
	return stamps
}

func stampAlive(stamp string, alive func(int) bool) bool {
	data, err := os.ReadFile(stamp) //nolint:gosec // G304: a pid file in a gotest-cli- temp dir
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	return err == nil && alive(pid)
}
