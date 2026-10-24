package drill_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/mvrahden/go-test/internal/drill"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// ScratchTestSuite: a drill builds at the same paths on every run of one
// checkout and shard, and only one drill at a time holds them.
//
// Sequential: a child forked meanwhile holds a lock until it execs.
type ScratchTestSuite struct{}

func (s *ScratchTestSuite) SuiteGuard() string {
	if runtime.GOOS != "linux" {
		return "the drill runs on Linux only"
	}
	return ""
}

func (s *ScratchTestSuite) AfterEach(t *gotest.T) { drill.ExportResetSeams() }

// staleScratch plants key's scratch under root, last used age ago.
func staleScratch(t *gotest.T, root, key string, age time.Duration) {
	gotest.NoError(t, os.MkdirAll(filepath.Join(root, key, "snapshot"), 0o700))
	lock := filepath.Join(root, key+".lock")
	gotest.NoError(t, os.WriteFile(lock, nil, 0o600))
	then := time.Now().Add(-age)
	gotest.NoError(t, os.Chtimes(lock, then, then))
}

func (s *ScratchTestSuite) TestTwoDrillsOnOneCheckoutUseTheSamePaths(t *gotest.T) {
	root, checkout := t.TempDir(), t.TempDir()
	first, err := drill.OpenScratch(root, checkout, 1, 2)
	gotest.NoError(t, err)
	leftover := filepath.Join(first.Dir, "snapshot")
	gotest.NoError(t, os.Mkdir(leftover, 0o700))
	// Released without clearing, as by a drill killed outright.
	gotest.NoError(t, first.Release())

	link := filepath.Join(t.TempDir(), "link")
	gotest.NoError(t, os.Symlink(checkout, link))
	second, err := drill.OpenScratch(root, link, 1, 2)
	gotest.NoError(t, err)
	gotest.Equal(t, first.Dir, second.Dir)
	entries, err := os.ReadDir(second.Dir)
	gotest.NoError(t, err)
	gotest.Empty(t, entries, "what the earlier drill left is cleared")
	gotest.NoError(t, second.Clear())
	gotest.NoError(t, second.Release())

	other, err := drill.OpenScratch(root, t.TempDir(), 1, 2)
	gotest.NoError(t, err)
	gotest.NotEqual(t, first.Dir, other.Dir)
	gotest.NoError(t, other.Release())
}

func (s *ScratchTestSuite) TestASecondDrillOnTheSameCheckoutAndShardIsRefused(t *gotest.T) {
	root, checkout := t.TempDir(), t.TempDir()
	held, err := drill.OpenScratch(root, checkout, 1, 2)
	gotest.NoError(t, err)
	_, err = drill.OpenScratch(root, checkout, 1, 2)
	gotest.ErrorContains(t, err, "another drill is running on")
	sibling, err := drill.OpenScratch(root, checkout, 2, 2)
	gotest.NoError(t, err, "another shard has a scratch of its own")
	gotest.NotEqual(t, held.Dir, sibling.Dir)
	gotest.NoError(t, sibling.Release())
	gotest.NoError(t, held.Release())
	again, err := drill.OpenScratch(root, checkout, 1, 2)
	gotest.NoError(t, err)
	gotest.NoError(t, again.Release())
}

func (s *ScratchTestSuite) TestTheDrillRefusesACheckoutAnotherDrillHolds(t *gotest.T) {
	cache, checkout := t.TempDir(), t.TempDir()
	held, err := drill.OpenScratch(filepath.Join(cache, "gotest", "drill"), checkout, 1, 1)
	gotest.NoError(t, err)
	cmd := childCommand("drill", "--root", checkout)
	cmd.Env = append(cmd.Env, "XDG_CACHE_HOME="+cache, "DRILL_SHARD=1/1", "DRILL_LOGS="+t.TempDir())
	out, _ := cmd.CombinedOutput()
	gotest.NoError(t, held.Release())
	gotest.Equal(t, 1, cmd.ProcessState.ExitCode())
	gotest.Regexp(t, `^DRILL  another drill is running on .* as shard 1/1 `, string(out))
}

func (s *ScratchTestSuite) TestACacheInsideTheCheckoutIsNotCopiedIntoItself(t *gotest.T) {
	checkout := t.TempDir()
	gotest.NoError(t, os.WriteFile(filepath.Join(checkout, "go.mod"), []byte("module example.com/x\n"), 0o600))
	cmd := childCommand("drill", "--root", checkout)
	cmd.Env = append(cmd.Env, "XDG_CACHE_HOME="+filepath.Join(checkout, ".cache"), "DRILL_SHARD=1/1", "DRILL_LOGS="+t.TempDir())
	out, _ := cmd.CombinedOutput()
	gotest.Equal(t, 1, cmd.ProcessState.ExitCode())
	gotest.Regexp(t, `(?m)^DRILL  could not build gotest; see bootstrap.log$`, string(out), "the snapshot is taken, so the bootstrap runs")
}

func (s *ScratchTestSuite) TestWithoutAUsableCacheDirTheScratchIsUnderTheTempDir(t *gotest.T) {
	entries := []struct {
		Desc, home string
	}{
		{"neither HOME nor XDG_CACHE_HOME", ""},
		{"a HOME it cannot write", "/"},
	}
	for t, entry := range gotest.Each(t, entries) {
		if entry.home == "/" && os.Getuid() == 0 {
			t.Skipf("root can write anywhere")
		}
		tmp := t.TempDir()
		t.Setenv("TMPDIR", tmp)
		t.Setenv("XDG_CACHE_HOME", "")
		t.Setenv("HOME", entry.home)
		gotest.NoError(t, os.Unsetenv("XDG_CACHE_HOME"))
		if entry.home == "" {
			gotest.NoError(t, os.Unsetenv("HOME"))
		}
		root, err := drill.ScratchRoot()
		gotest.NoError(t, err)
		gotest.Equal(t, filepath.Join(tmp, "gotest-drill-"+strconv.Itoa(os.Getuid())), root)
		scratch, err := drill.OpenScratch(root, t.TempDir(), 1, 1)
		gotest.NoError(t, err)
		gotest.NoError(t, scratch.Release())
		again, err := drill.ScratchRoot()
		gotest.NoError(t, err)
		gotest.Equal(t, root, again, "the fallback is fixed, so reruns hit the build cache")
	}
}

func (s *ScratchTestSuite) TestATempDirFallbackSomeoneLinkedElsewhereIsRefused(t *gotest.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("HOME", "")
	gotest.NoError(t, os.Unsetenv("XDG_CACHE_HOME"))
	gotest.NoError(t, os.Unsetenv("HOME"))
	planted := filepath.Join(tmp, "gotest-drill-"+strconv.Itoa(os.Getuid()))
	gotest.NoError(t, os.Symlink(t.TempDir(), planted))
	_, err := drill.ScratchRoot()
	gotest.ErrorContains(t, err, planted+" is not a directory of this user's")
}

func (s *ScratchTestSuite) TestOpeningPrunesScratchesUnusedForAWeek(t *gotest.T) {
	root := t.TempDir()
	staleScratch(t, root, "old-1-of-1", 8*24*time.Hour)
	staleScratch(t, root, "recent-1-of-1", 6*24*time.Hour)
	held, err := drill.OpenScratch(root, t.TempDir(), 1, 1)
	gotest.NoError(t, err)
	week := time.Now().Add(-8 * 24 * time.Hour)
	gotest.NoError(t, os.Chtimes(held.Lock().Name(), week, week))
	gotest.NoError(t, os.Mkdir(filepath.Join(held.Dir, "snapshot"), 0o700))

	opened, err := drill.OpenScratch(root, t.TempDir(), 1, 1)
	gotest.NoError(t, err)
	_, err = os.Stat(filepath.Join(root, "old-1-of-1"))
	gotest.ErrorIs(t, err, os.ErrNotExist, "an unused scratch is pruned")
	_, err = os.Stat(filepath.Join(root, "old-1-of-1.lock"))
	gotest.ErrorIs(t, err, os.ErrNotExist, "and its lock file with it")
	_, err = os.Stat(filepath.Join(root, "recent-1-of-1", "snapshot"))
	gotest.NoError(t, err, "a scratch used this week stays")
	_, err = os.Stat(filepath.Join(held.Dir, "snapshot"))
	gotest.NoError(t, err, "a scratch a drill holds stays, however old")
	gotest.NoError(t, opened.Release())
	gotest.NoError(t, held.Release())
}

func (s *ScratchTestSuite) TestADrillOpeningWhileItsScratchIsPrunedIsNotRefused(t *gotest.T) {
	root, checkout, other := t.TempDir(), t.TempDir(), t.TempDir()
	old, err := drill.OpenScratch(root, checkout, 1, 1)
	gotest.NoError(t, err)
	gotest.NoError(t, os.Mkdir(filepath.Join(old.Dir, "snapshot"), 0o700))
	week := time.Now().Add(-8 * 24 * time.Hour)
	gotest.NoError(t, os.Chtimes(old.Lock().Name(), week, week))
	gotest.NoError(t, old.Release())

	// The first removal waits; the drill opening meanwhile may prune too.
	entered, release := make(chan struct{}), make(chan struct{})
	var first atomic.Bool
	drill.ExportSetRemoveScratch(func(path string) error {
		if first.CompareAndSwap(false, true) {
			close(entered)
			<-release
		}
		return drill.ExportRemoveAll(path)
	})
	wait := gotest.Go(t, func() {
		pruning, err := drill.OpenScratch(root, other, 1, 1)
		gotest.NoError(t, err)
		gotest.NoError(t, pruning.Release())
	})
	select {
	case <-entered:
	case <-time.After(30 * time.Second):
		close(release)
		gotest.Fail(t, "the old scratch was never pruned")
	}
	opened, err := drill.OpenScratch(root, checkout, 1, 1)
	close(release)
	wait()
	gotest.NoError(t, err, "a slow prune of its scratch refused a drill")
	_, err = os.Stat(opened.Dir)
	gotest.NoError(t, err, "the prune removed the scratch the drill opened")
	gotest.NoError(t, opened.Release())
}

func (s *ScratchTestSuite) TestADrillThatOpenedALockFileAPruneThenRemovedIsNotRefused(t *gotest.T) {
	root, checkout := t.TempDir(), t.TempDir()
	var pruned atomic.Bool
	var pruner *os.File
	drill.ExportSetLockOpened(func(path string) {
		if !pruned.CompareAndSwap(false, true) {
			return
		}
		// A prune locks the file the drill just opened, and removes it.
		var err error
		pruner, err = os.Open(path)
		gotest.NoError(t, err)
		gotest.NoError(t, lockFile(pruner))
		gotest.NoError(t, os.Remove(path))
	})
	opened, err := drill.OpenScratch(root, checkout, 1, 1)
	gotest.NoError(t, pruner.Close())
	gotest.NoError(t, err, "a prune of the lock file refused a drill")
	gotest.NoError(t, opened.Release())
}

func (s *ScratchTestSuite) TestOpeningRemovesWhatAnInterruptedPruneLeft(t *gotest.T) {
	root := t.TempDir()
	staleScratch(t, root, "old-1-of-1", 8*24*time.Hour)
	var trash string
	drill.ExportSetRemoveScratch(func(path string) error {
		trash = path
		return nil // the prune is killed here
	})
	first, err := drill.OpenScratch(root, t.TempDir(), 1, 1)
	gotest.NoError(t, err)
	gotest.NoError(t, first.Release())
	gotest.NotZero(t, trash)
	_, err = os.Stat(trash)
	gotest.NoError(t, err)
	drill.ExportResetSeams()

	second, err := drill.OpenScratch(root, t.TempDir(), 1, 1)
	gotest.NoError(t, err)
	gotest.NoError(t, second.Release())
	_, err = os.Stat(trash)
	gotest.ErrorIs(t, err, os.ErrNotExist, "what the interrupted prune left is removed")
}

func (s *ScratchTestSuite) TestAPruneSparesALockFileReplacedBeforeItLocks(t *gotest.T) {
	root := t.TempDir()
	staleScratch(t, root, "old-1-of-1", 8*24*time.Hour)
	lock := filepath.Join(root, "old-1-of-1.lock")
	var once sync.Once
	drill.ExportSetLockOpened(func(path string) {
		if path != lock {
			return
		}
		// Another prune removed it, and a new drill took the scratch.
		once.Do(func() {
			gotest.NoError(t, os.Remove(lock))
			gotest.NoError(t, os.WriteFile(lock, nil, 0o600))
		})
	})
	opened, err := drill.OpenScratch(root, t.TempDir(), 1, 1)
	gotest.NoError(t, err)
	gotest.NoError(t, opened.Release())
	_, err = os.Stat(filepath.Join(root, "old-1-of-1", "snapshot"))
	gotest.NoError(t, err, "the new drill's scratch was pruned")
	_, err = os.Stat(lock)
	gotest.NoError(t, err, "the new drill's lock file was pruned")
}

func (s *ScratchTestSuite) TestAPruneSparesAScratchUsedAfterItLooked(t *gotest.T) {
	root := t.TempDir()
	staleScratch(t, root, "old-1-of-1", 8*24*time.Hour)
	lock := filepath.Join(root, "old-1-of-1.lock")
	drill.ExportSetLockOpened(func(path string) {
		if path == lock {
			now := time.Now()
			gotest.NoError(t, os.Chtimes(lock, now, now))
		}
	})
	opened, err := drill.OpenScratch(root, t.TempDir(), 1, 1)
	gotest.NoError(t, err)
	gotest.NoError(t, opened.Release())
	_, err = os.Stat(filepath.Join(root, "old-1-of-1", "snapshot"))
	gotest.NoError(t, err, "a scratch used since the prune looked was pruned")
}

func (s *ScratchTestSuite) TestATempDirFallbackOthersCanWriteIsRefused(t *gotest.T) {
	entries := []struct {
		Desc         string
		parent, root os.FileMode
		refused      string
	}{
		{"a temp dir anyone can rename in", 0o777, 0, " is writable by anyone and not sticky"},
		{"a sticky temp dir", 0o777 | os.ModeSticky, 0, ""},
		{"a fallback others can write", 0o700, 0o770, " is writable by others"},
	}
	for t, entry := range gotest.Each(t, entries) {
		tmp := t.TempDir()
		gotest.NoError(t, os.Chmod(tmp, entry.parent))
		t.Setenv("TMPDIR", tmp)
		t.Setenv("XDG_CACHE_HOME", "")
		t.Setenv("HOME", "")
		gotest.NoError(t, os.Unsetenv("XDG_CACHE_HOME"))
		gotest.NoError(t, os.Unsetenv("HOME"))
		fallback := filepath.Join(tmp, "gotest-drill-"+strconv.Itoa(os.Getuid()))
		if entry.root != 0 {
			gotest.NoError(t, os.Mkdir(fallback, 0o700))
			gotest.NoError(t, os.Chmod(fallback, entry.root))
		}
		root, err := drill.ScratchRoot()
		switch {
		case entry.refused == "":
			gotest.NoError(t, err)
			gotest.Equal(t, fallback, root)
		case entry.root != 0:
			gotest.ErrorContains(t, err, fallback+entry.refused)
		default:
			gotest.ErrorContains(t, err, tmp+entry.refused)
		}
	}
}

func (s *ScratchTestSuite) TestAFilesystemThatCannotLockIsNamed(t *gotest.T) {
	for _, errno := range []syscall.Errno{syscall.ENOLCK, syscall.EOPNOTSUPP} {
		err := drill.ExportLockError("/nfs/drill/x.lock", errno)
		gotest.ErrorContains(t, err, "cannot lock /nfs/drill/x.lock: its filesystem cannot lock files")
		gotest.ErrorIs(t, err, errno)
	}
}

// ScratchLockTestSuite: the scratch stays locked while anything a drill
// started can still write in it, even once the drill itself is gone.
type ScratchLockTestSuite struct{}

func (s *ScratchLockTestSuite) SuiteGuard() string {
	if runtime.GOOS != "linux" {
		return "the drill runs on Linux only"
	}
	return ""
}

func (s *ScratchLockTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *ScratchLockTestSuite) BeforeEach(t *gotest.T) *procs { return newProcs(t) }

func (s *ScratchLockTestSuite) AfterEach(t *gotest.T, p *procs) { p.stop() }

// dead reports whether pid has exited, a zombie included.
func dead(pid int) bool {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return true
	}
	s := string(data)
	fields := strings.Fields(s[strings.LastIndexByte(s, ')')+1:])
	return len(fields) > 0 && (fields[0] == "Z" || fields[0] == "X")
}

func kill(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Kill()
}

// parent is pid's parent.
func parent(t *gotest.T, pid int) int {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	gotest.NoError(t, err)
	s := string(data)
	fields := strings.Fields(s[strings.LastIndexByte(s, ')')+1:])
	ppid, err := strconv.Atoi(fields[1])
	gotest.NoError(t, err)
	return ppid
}

func (s *ScratchLockTestSuite) TestARefusalNamesTheProcessesHoldingTheLock(t *gotest.T, p *procs) {
	root, checkout := t.TempDir(), t.TempDir()
	cmd := childCommand("hold", root, checkout, p.dir, filepath.Join(p.dir, "pid"))
	p.start(t, cmd)
	sleeper := p.pid(t, filepath.Join(p.dir, "pid"))
	supervisor := parent(t, sleeper)
	p.mu.Lock()
	p.pids = append(p.pids, supervisor)
	p.mu.Unlock()
	_, err := drill.OpenScratch(root, checkout, 1, 1)
	gotest.ErrorContains(t, err, "another drill is running on")
	_, named, _ := strings.Cut(err.Error(), "held by ")
	for _, pid := range []int{cmd.Process.Pid, supervisor, sleeper} {
		gotest.Regexp(t, `^pids? ([0-9]+, )*`+strconv.Itoa(pid)+`\b`, named, "pid %d holds the lock", pid)
	}
}

func (s *ScratchLockTestSuite) TestAKilledDrillsScratchStaysLockedWhileItsDescendantsRun(t *gotest.T, p *procs) {
	root, checkout := t.TempDir(), t.TempDir()
	cmd := childCommand("hold", root, checkout, p.dir, filepath.Join(p.dir, "pid"))
	holder := p.start(t, cmd)
	sleeper := p.pid(t, filepath.Join(p.dir, "pid"))
	supervisor := parent(t, sleeper)
	p.mu.Lock()
	p.pids = append(p.pids, supervisor)
	p.mu.Unlock()
	refused := func(why string) {
		_, err := drill.OpenScratch(root, checkout, 1, 1)
		gotest.ErrorContains(t, err, "another drill is running on", why)
	}

	gotest.NoError(t, cmd.Process.Kill())
	holder.wait(t, 30*time.Second)
	refused("the drill is gone, its supervisor runs")

	gotest.NoError(t, kill(supervisor))
	gotest.Eventually(t, 30*time.Second, 10*time.Millisecond, func(poll *gotest.R) {
		gotest.True(poll, dead(supervisor), "the supervisor outlived SIGKILL")
	})
	refused("the supervisor is gone, its command runs")

	gotest.NoError(t, kill(sleeper))
	gotest.Eventually(t, 30*time.Second, 10*time.Millisecond, func(poll *gotest.R) {
		scratch, err := drill.OpenScratch(root, checkout, 1, 1)
		gotest.NoError(poll, err, "the last holder is gone")
		gotest.NoError(poll, scratch.Release())
	})
}

// bootstrap is what the first command a drill ran saw: its directory and
// the files it held open, beside the drill's scratch and lock file.
type bootstrap struct {
	dir, scratch, lock string
	open               []string
}

// runBootstrap runs the drill on a checkout with one mutant, under a go on
// PATH that records what it sees and fails, so the drill ends at its first
// build.
func runBootstrap(t *gotest.T, p *procs) bootstrap {
	checkout, cache, bin, record := t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir()
	mutants := filepath.Join(checkout, "tests", "drill", "mutants")
	gotest.NoError(t, os.MkdirAll(mutants, 0o700))
	gotest.NoError(t, os.WriteFile(filepath.Join(mutants, "m.patch"), nil, 0o600))
	script := "#!/bin/sh\n[ -e \"$REC/dir\" ] && exit 1\nfor fd in /proc/$$/fd/*; do readlink \"$fd\"; done > \"$REC/open\"\npwd -P > \"$REC/dir\"\nexit 1\n"
	gotest.NoError(t, os.WriteFile(filepath.Join(bin, "go"), []byte(script), 0o700)) //nolint:gosec // G306: an executable
	cmd := childCommand("drill", "--root", checkout, "m")
	cmd.Env = append(cmd.Env, "XDG_CACHE_HOME="+cache, "PATH="+bin+":"+os.Getenv("PATH"), "REC="+record,
		"DRILL_LOGS="+p.dir, "DRILL_JOBS=1", "DRILL_SHARD=1/1")
	gotest.NotZero(t, p.start(t, cmd).wait(t, 2*time.Minute), "a drill whose build fails succeeded")
	dir, err := os.ReadFile(filepath.Join(record, "dir"))
	gotest.NoError(t, err, "the drill ran no go")
	open, err := os.ReadFile(filepath.Join(record, "open"))
	gotest.NoError(t, err)
	locks, err := filepath.Glob(filepath.Join(cache, "gotest", "drill", "*.lock"))
	gotest.NoError(t, err)
	gotest.Len(t, locks, 1)
	lock, err := filepath.EvalSymlinks(locks[0])
	gotest.NoError(t, err)
	return bootstrap{
		dir: strings.TrimSpace(string(dir)), scratch: strings.TrimSuffix(lock, ".lock"), lock: lock,
		open: strings.Fields(string(open)),
	}
}

func (s *ScratchLockTestSuite) TestADrillBuildsInItsCheckoutsScratch(t *gotest.T, p *procs) {
	b := runBootstrap(t, p)
	gotest.Equal(t, filepath.Join(b.scratch, "snapshot"), b.dir)
}

func (s *ScratchLockTestSuite) TestWhatADrillRunsHoldsItsScratchLock(t *gotest.T, p *procs) {
	b := runBootstrap(t, p)
	gotest.Contains(t, b.open, b.lock)
}
