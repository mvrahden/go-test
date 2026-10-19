package testkit_test

import (
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// deadPID is above every platform's pid ceiling, so no process holds it.
const deadPID = "999999999"

// CLIDirTestSuite: building the CLI sweeps the CLI dirs gone processes left,
// and a CLI built once from the same sources is reused, not built again.
// Sequential: the methods move the process's temp dir.
type CLIDirTestSuite struct{}

func (s *CLIDirTestSuite) TestStaleCLIDirsAreSwept(t *gotest.T) {
	tmp := privateTempDir(t)
	dead := cliDir(t, tmp, "gotest-cli-dead", ".pid", deadPID)
	idleBuild := publishedBuild(t, tmp, "gotest-cli-key-idle")
	staleBuild := publishedBuild(t, tmp, "gotest-cli-key-stale")
	live := cliDir(t, tmp, "gotest-cli-live", ".pid", strconv.Itoa(os.Getpid()))
	fresh := cliDir(t, tmp, "gotest-cli-fresh", "", "")
	old := cliDir(t, tmp, "gotest-cli-old", "", "")
	orphanLock := filepath.Join(tmp, "gotest-cli-key-orphan.lock")
	write(t, orphanLock, "")
	aged := time.Now().Add(-48 * time.Hour) // past the grace an unstamped dir gets
	gotest.NoError(t, os.Chtimes(old, aged, aged))
	gotest.NoError(t, os.Chtimes(staleBuild, aged, aged))

	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	binary, err := testkit.ExportResolveCLI(t.Context(), root)
	gotest.NoError(t, err)

	t.When("testkit builds its CLI", func(w *gotest.T) {
		w.It("removes a dir whose process is gone", func(it *gotest.T) {
			gotest.False(it, exists(dead))
		})
		w.It("removes a reusable build no live process used for a while", func(it *gotest.T) {
			gotest.False(it, exists(staleBuild))
		})
		w.It("removes the lock of a build it removed", func(it *gotest.T) {
			gotest.False(it, exists(staleBuild+".lock"))
		})
		w.It("removes a lock whose build is gone", func(it *gotest.T) {
			gotest.False(it, exists(orphanLock))
		})
		w.It("keeps a reusable build used moments ago, for the next process", func(it *gotest.T) {
			gotest.True(it, exists(idleBuild))
		})
		w.It("removes a pid-less dir once it is old enough", func(it *gotest.T) {
			gotest.False(it, exists(old))
		})
		w.It("keeps a dir whose process is alive", func(it *gotest.T) {
			gotest.True(it, exists(live))
		})
		w.It("keeps a pid-less dir too new to have been stamped", func(it *gotest.T) {
			gotest.True(it, exists(fresh))
		})
		w.It("builds into a dir of its own, stamped with its pid", func(it *gotest.T) {
			gotest.Regexp(it, "^gotest-cli-", filepath.Base(filepath.Dir(binary)))
			gotest.True(it, exists(binary))
			gotest.True(it, testkit.ExportCLIDirAlive(filepath.Dir(binary)))
		})
	})
}

func (s *CLIDirTestSuite) TestABuiltCLIIsReused(t *gotest.T) {
	tmp := privateTempDir(t)
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	first, err := testkit.ExportResolveCLI(t.Context(), root)
	gotest.NoError(t, err)
	before := cliDirs(t, tmp)
	second, err := testkit.ExportResolveCLI(t.Context(), root)
	gotest.NoError(t, err)

	t.When("the sources are unchanged", func(w *gotest.T) {
		w.It("hands out the same binary", func(it *gotest.T) {
			gotest.Equal(it, first, second)
		})
		w.It("creates no further dir", func(it *gotest.T) {
			gotest.Equal(it, before, cliDirs(it, tmp))
		})
	})
}

func (s *CLIDirTestSuite) TestTheKeyFollowsTheSources(t *gotest.T) {
	mod := t.TempDir()
	main := filepath.Join(mod, "cmd", "gotest", "main.go")
	note := filepath.Join(mod, "cmd", "gotest", "note.txt")
	gotest.NoError(t, os.MkdirAll(filepath.Dir(main), 0o750))
	write(t, filepath.Join(mod, "go.mod"), "module keymod\n\ngo 1.26.0\n")
	src := "package main\n\nimport _ \"embed\"\n\n//go:embed note.txt\nvar note string\n\nfunc main() { println(note) }\n"
	write(t, main, src)
	write(t, note, "one")
	base, err := testkit.ExportCLIKey(t.Context(), mod)
	gotest.NoError(t, err)

	t.When("nothing changed", func(w *gotest.T) {
		again, err := testkit.ExportCLIKey(w.Context(), mod)
		w.It("keys the same", func(it *gotest.T) {
			gotest.NoError(it, err)
			gotest.Equal(it, base, again)
		})
	})
	t.When("a Go file changed", func(w *gotest.T) {
		write(w, main, src+"\nfunc unused() {}\n")
		changed, err := testkit.ExportCLIKey(w.Context(), mod)
		write(w, main, src)
		w.It("keys differently", func(it *gotest.T) {
			gotest.NoError(it, err)
			gotest.NotEqual(it, base, changed)
		})
	})
	t.When("an embedded file changed", func(w *gotest.T) {
		write(w, note, "two")
		changed, err := testkit.ExportCLIKey(w.Context(), mod)
		write(w, note, "one")
		w.It("keys differently", func(it *gotest.T) {
			gotest.NoError(it, err)
			gotest.NotEqual(it, base, changed)
		})
	})
	t.When("the same sources sit in another tree", func(w *gotest.T) {
		other := w.TempDir()
		gotest.NoError(w, os.MkdirAll(filepath.Join(other, "cmd", "gotest"), 0o750))
		write(w, filepath.Join(other, "go.mod"), "module keymod\n\ngo 1.26.0\n")
		write(w, filepath.Join(other, "cmd", "gotest", "main.go"), src)
		write(w, filepath.Join(other, "cmd", "gotest", "note.txt"), "one")
		copied, err := testkit.ExportCLIKey(w.Context(), other)
		w.It("keys differently", func(it *gotest.T) {
			gotest.NoError(it, err)
			gotest.NotEqual(it, base, copied)
		})
	})
}

func (s *CLIDirTestSuite) TestASweepNeverTakesAClaimedBuild(t *gotest.T) {
	tmp := privateTempDir(t)
	keyed := filepath.Join(tmp, testkit.ExportKeyedPrefix+"stress")
	// Fake pids, far above any real one, live while their goroutine says so.
	var live sync.Map
	alive := func(pid int) bool { _, ok := live.Load(pid); return ok }
	var nextPID atomic.Int64
	nextPID.Store(2_000_000_000)
	pid := func() int { p := int(nextPID.Add(1)); live.Store(p, true); return p }

	var stop atomic.Bool
	var published, claimed, vanished atomic.Int64
	var wg sync.WaitGroup
	// The publisher puts a build back whenever a sweep took it; its stamp
	// dies once it is published, as a builder's does when it exits.
	wg.Go(func() {
		for !stop.Load() {
			if exists(keyed) {
				continue
			}
			builder := pid()
			dir, err := testkit.ExportNewStampedDir(builder)
			if err == nil && os.WriteFile(filepath.Join(dir, testkit.ExportCLIName()), []byte("cli"), 0o600) == nil {
				if testkit.ExportPublish(dir, keyed) {
					published.Add(1)
				} else {
					_ = os.RemoveAll(dir)
				}
			}
			live.Delete(builder)
		}
	})
	for range 4 {
		wg.Go(func() {
			for !stop.Load() {
				testkit.ExportSweepDir(keyed, alive, 0)
			}
		})
	}
	for range 8 {
		wg.Go(func() {
			for !stop.Load() {
				me := pid()
				if binary, ok := testkit.ExportClaimAs(keyed, me, alive); ok {
					claimed.Add(1)
					for range 20 {
						if !exists(binary) {
							vanished.Add(1)
							break
						}
					}
				}
				live.Delete(me)
			}
		})
	}
	time.Sleep(2 * time.Second)
	stop.Store(true)
	wg.Wait()

	t.When("sweeps race claims of the same build", func(w *gotest.T) {
		w.It("claims the build, and sweeps take it between claims", func(it *gotest.T) {
			gotest.Greater(it, claimed.Load(), int64(0))
			gotest.Greater(it, published.Load(), int64(1))
		})
		w.It("never takes a build from a live claimer", func(it *gotest.T) {
			gotest.Zero(it, vanished.Load(), "%d of %d claims lost their binary", vanished.Load(), claimed.Load())
		})
	})
}

// privateTempDir points the process's temp dir at a fresh one for the test.
func privateTempDir(t *gotest.T) string {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	t.Setenv("TMP", tmp)
	t.Setenv("TEMP", tmp)
	return tmp
}

// cliDir makes tmp/name, with a pid file holding pid when pidFile is given.
func cliDir(t *gotest.T, tmp, name, pidFile, pid string) string {
	dir := filepath.Join(tmp, name)
	gotest.NoError(t, os.Mkdir(dir, 0o750))
	if pidFile != "" {
		write(t, filepath.Join(dir, pidFile), pid)
	}
	return dir
}

// publishedBuild makes a build published as tmp/name whose users are gone.
func publishedBuild(t *gotest.T, tmp, name string) string {
	dir := cliDir(t, tmp, name, ".pid."+deadPID, deadPID)
	write(t, dir+".lock", "")
	write(t, filepath.Join(dir, testkit.ExportCLIName()), "cli")
	return dir
}

func cliDirs(t *gotest.T, tmp string) []string {
	dirs, err := filepath.Glob(filepath.Join(tmp, "gotest-cli-*"))
	gotest.NoError(t, err)
	return dirs
}

func write(t *gotest.T, path, data string) {
	gotest.NoError(t, os.WriteFile(path, []byte(data), 0o600))
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
