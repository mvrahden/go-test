package gotestrunner_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/about"
	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// OverlayTestSuite covers overlay generation, the run's own overlay dir and the sweep.
// Sequential: Setenv (the cache directory).
type OverlayTestSuite struct{}

// overlayDirsCtx collects the directories a test creates, for AfterEach to
// remove, and what restores the owner readers a test stood in for.
type overlayDirsCtx struct {
	dirs    []string
	restore func()
}

// BeforeEach gives every method a temp dir and cache of its own: its sweeps
// never reach those of other runs.
func (s *OverlayTestSuite) BeforeEach(t *gotest.T) *overlayDirsCtx {
	testkit.IsolateTempDir(t)
	t.Setenv(protocol.EnvCacheDir, t.TempDir())
	return &overlayDirsCtx{}
}

func (s *OverlayTestSuite) AfterEach(t *gotest.T, ctx *overlayDirsCtx) {
	if ctx.restore != nil {
		ctx.restore()
	}
	for _, dir := range ctx.dirs {
		gotest.NoError(t, os.RemoveAll(dir))
	}
}

func (s *OverlayTestSuite) TestOverlayManagement(t *gotest.T, ctx *overlayDirsCtx) {
	t.When("writing overlay", func(w *gotest.T) {
		w.It("creates correct overlay entries for PTest and PXTest", func(it *gotest.T) {
			results := gotestgen.GenerateResults{
				{AbsPath: "/fake/pkg/a", PTest: []byte("package a\n"), PXTest: []byte("package a_test\n")},
				{AbsPath: "/fake/pkg/b", PTest: []byte("package b\n")},
			}

			tmpDir, err := gotestrunner.WriteOverlay(results)
			gotest.NoError(it, err)
			ctx.dirs = append(ctx.dirs, tmpDir)

			data, err := os.ReadFile(filepath.Join(tmpDir, "overlay.json"))
			gotest.NoError(it, err)
			var ov gotestrunner.ExportOverlayJSON
			err = json.Unmarshal(data, &ov)
			gotest.NoError(it, err)

			wantKeys := map[string]bool{
				filepath.Join("/fake/pkg/a", about.PSuite):  true,
				filepath.Join("/fake/pkg/a", about.PXSuite): true,
				filepath.Join("/fake/pkg/b", about.PSuite):  true,
			}
			gotest.Len(it, wantKeys, len(ov.Replace))
			for virtual, real := range ov.Replace {
				gotest.True(it, wantKeys[virtual], "unexpected overlay key: %s", virtual)
				_, err := os.Stat(real)
				gotest.NoError(it, err)
			}

			bPXSuite := filepath.Join("/fake/pkg/b", about.PXSuite)
			_, ok := ov.Replace[bPXSuite]
			gotest.False(it, ok, "pkg/b should not have PXSuite mapping (empty PXTest)")
		})

		w.It("replaces each generated file left on disk it does not regenerate with its package clause", func(it *gotest.T) {
			pkgDir := it.TempDir()
			stale := "package a\n\nfunc TestOldTestSuite() { _ = OldTestSuite{} }\n"
			for _, name := range []string{about.PSuite, about.PXSuite} {
				gotest.NoError(it, os.WriteFile(filepath.Join(pkgDir, name), []byte(stale), 0o600))
			}
			results := gotestgen.GenerateResults{{AbsPath: pkgDir, PTest: []byte("package a\n// fresh\n")}}

			tmpDir, err := gotestrunner.WriteOverlay(results)
			gotest.NoError(it, err)
			ctx.dirs = append(ctx.dirs, tmpDir)

			data, err := os.ReadFile(filepath.Join(tmpDir, "overlay.json"))
			gotest.NoError(it, err)
			var ov gotestrunner.ExportOverlayJSON
			gotest.NoError(it, json.Unmarshal(data, &ov))
			gotest.Len(it, ov.Replace, 2)
			for name, want := range map[string]string{
				about.PSuite:  "package a\n// fresh\n",
				about.PXSuite: "package a\n",
			} {
				got, err := os.ReadFile(ov.Replace[filepath.Join(pkgDir, name)])
				gotest.NoError(it, err, name)
				gotest.Equal(it, want, string(got), name)
			}
		})

		w.It("produces unique overlay directories when called twice", func(it *gotest.T) {
			results := gotestgen.GenerateResults{
				{AbsPath: "/fake/pkg/a", PTest: []byte("package a\n")},
			}

			dir1, err := gotestrunner.WriteOverlay(results)
			gotest.NoError(it, err)
			ctx.dirs = append(ctx.dirs, dir1)

			dir2, err := gotestrunner.WriteOverlay(results)
			gotest.NoError(it, err)
			ctx.dirs = append(ctx.dirs, dir2)

			gotest.NotEqual(it, dir1, dir2)
		})

		w.It("contains a .pid file with the current PID", func(it *gotest.T) {
			results := gotestgen.GenerateResults{
				{AbsPath: "/fake/pkg/a", PTest: []byte("package a\n")},
			}

			tmpDir, err := gotestrunner.WriteOverlay(results)
			gotest.NoError(it, err)
			ctx.dirs = append(ctx.dirs, tmpDir)

			data, err := os.ReadFile(filepath.Join(tmpDir, ".pid"))
			gotest.NoError(it, err)

			pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
			gotest.NoError(it, err)
			gotest.Equal(it, os.Getpid(), pid)
		})

		w.It("creates an empty overlay for nil results", func(it *gotest.T) {
			tmpDir, err := gotestrunner.WriteOverlay(nil)
			gotest.NoError(it, err)
			ctx.dirs = append(ctx.dirs, tmpDir)

			data, err := os.ReadFile(filepath.Join(tmpDir, "overlay.json"))
			gotest.NoError(it, err)
			var ov gotestrunner.ExportOverlayJSON
			err = json.Unmarshal(data, &ov)
			gotest.NoError(it, err)
			gotest.Empty(it, ov.Replace)
		})
	})

	t.When("cleaning stale overlays", func(w *gotest.T) {
		w.It("removes overlay with dead PID", func(it *gotest.T) {
			dir, err := os.MkdirTemp(os.TempDir(), "gotest-overlay-test-")
			gotest.NoError(it, err)
			ctx.dirs = append(ctx.dirs, dir)
			// Write a PID that doesn't exist (use a very high PID)
			_ = os.WriteFile(filepath.Join(dir, ".pid"), []byte("999999999"), 0600)

			gotestrunner.CleanStaleOverlays()

			_, err = os.Stat(dir)
			gotest.True(it, os.IsNotExist(err), "expected stale overlay to be removed")
		})

		w.It("keeps overlay with live PID", func(it *gotest.T) {
			dir, err := os.MkdirTemp(os.TempDir(), "gotest-overlay-test-")
			gotest.NoError(it, err)
			ctx.dirs = append(ctx.dirs, dir)

			// Write our own PID -- guaranteed alive
			_ = os.WriteFile(filepath.Join(dir, ".pid"), []byte(strconv.Itoa(os.Getpid())), 0600)

			gotestrunner.CleanStaleOverlays()

			_, err = os.Stat(dir)
			gotest.False(it, os.IsNotExist(err), "expected live overlay to be kept")
		})

		w.It("keeps a fresh overlay with no PID file: its writer has not written the pid yet", func(it *gotest.T) {
			dir, err := os.MkdirTemp(os.TempDir(), "gotest-overlay-test-")
			gotest.NoError(it, err)
			ctx.dirs = append(ctx.dirs, dir)

			gotestrunner.CleanStaleOverlays()

			_, err = os.Stat(dir)
			gotest.NoError(it, err, "expected the fresh overlay to be kept")
		})

		w.It("removes an overlay with no PID file once it is older than a minute", func(it *gotest.T) {
			dir, err := os.MkdirTemp(os.TempDir(), "gotest-overlay-test-")
			gotest.NoError(it, err)
			ctx.dirs = append(ctx.dirs, dir)
			old := time.Now().Add(-2 * time.Minute)
			gotest.NoError(it, os.Chtimes(dir, old, old))

			gotestrunner.CleanStaleOverlays()

			_, err = os.Stat(dir)
			gotest.True(it, os.IsNotExist(err), "expected overlay without PID file to be removed")
		})

		w.It("removes a work dir whose owner is dead", func(it *gotest.T) {
			dir, err := os.MkdirTemp(os.TempDir(), "gotest-work-")
			gotest.NoError(it, err)
			ctx.dirs = append(ctx.dirs, dir)
			_ = os.WriteFile(filepath.Join(dir, ".pid"), []byte("999999999"), 0600)

			gotestrunner.CleanStaleOverlays()

			_, err = os.Stat(dir)
			gotest.True(it, os.IsNotExist(err), "expected the dead run's work dir to be removed")
		})

		w.It("keeps a fresh work dir with no pid file: an earlier gotest, which wrote none, may still be running in it", func(it *gotest.T) {
			dir, err := os.MkdirTemp(os.TempDir(), "gotest-work-")
			gotest.NoError(it, err)
			ctx.dirs = append(ctx.dirs, dir)

			gotestrunner.CleanStaleOverlays()

			_, err = os.Stat(dir)
			gotest.NoError(it, err, "expected the fresh work dir to be kept")
		})

		w.It("keeps a fresh work dir whose pid file is empty, as an earlier gotest's unfinished write leaves it", func(it *gotest.T) {
			dir, err := os.MkdirTemp(os.TempDir(), "gotest-work-")
			gotest.NoError(it, err)
			ctx.dirs = append(ctx.dirs, dir)
			gotest.NoError(it, os.WriteFile(filepath.Join(dir, ".pid"), nil, 0o600))

			gotestrunner.CleanStaleOverlays()

			_, err = os.Stat(dir)
			gotest.NoError(it, err, "expected the fresh work dir to be kept")
		})

		w.It("keeps a work dir marked to be kept, even once its owner is dead", func(it *gotest.T) {
			dir, err := os.MkdirTemp(os.TempDir(), "gotest-work-")
			gotest.NoError(it, err)
			ctx.dirs = append(ctx.dirs, dir)
			gotest.NoError(it, os.WriteFile(filepath.Join(dir, ".pid"), []byte("999999999"), 0o600))
			gotest.NoError(it, os.WriteFile(filepath.Join(dir, ".keep"), nil, 0o600))

			gotestrunner.CleanStaleOverlays()

			_, err = os.Stat(dir)
			gotest.NoError(it, err, "expected the marked work dir to be kept")
		})

		w.It("removes a work dir marked to be kept once it is older than 7 days", func(it *gotest.T) {
			dir, err := os.MkdirTemp(os.TempDir(), "gotest-work-")
			gotest.NoError(it, err)
			ctx.dirs = append(ctx.dirs, dir)
			gotest.NoError(it, os.WriteFile(filepath.Join(dir, ".pid"), []byte("999999999"), 0o600))
			gotest.NoError(it, os.WriteFile(filepath.Join(dir, ".keep"), nil, 0o600))
			then := time.Now().Add(-8 * 24 * time.Hour)
			gotest.NoError(it, os.Chtimes(dir, then, then))

			gotestrunner.CleanStaleOverlays()

			_, err = os.Stat(dir)
			gotest.True(it, os.IsNotExist(err), "expected the aged marked work dir to be removed")
		})

		w.It("keeps a work dir with no pid file that is two days old: an earlier gotest's session may run that long", func(it *gotest.T) {
			dir, err := os.MkdirTemp(os.TempDir(), "gotest-work-")
			gotest.NoError(it, err)
			ctx.dirs = append(ctx.dirs, dir)
			old := time.Now().Add(-48 * time.Hour)
			gotest.NoError(it, os.Chtimes(dir, old, old))

			gotestrunner.CleanStaleOverlays()

			_, err = os.Stat(dir)
			gotest.NoError(it, err, "expected the two-day-old work dir to be kept")
		})

		w.It("removes a work dir with no pid file once it is older than 7 days", func(it *gotest.T) {
			dir, err := os.MkdirTemp(os.TempDir(), "gotest-work-")
			gotest.NoError(it, err)
			ctx.dirs = append(ctx.dirs, dir)
			old := time.Now().Add(-8 * 24 * time.Hour)
			gotest.NoError(it, os.Chtimes(dir, old, old))

			gotestrunner.CleanStaleOverlays()

			_, err = os.Stat(dir)
			gotest.True(it, os.IsNotExist(err), "expected the aged work dir to be removed")
		})

		w.It("keeps a work dir whose owner is alive", func(it *gotest.T) {
			dir, err := os.MkdirTemp(os.TempDir(), "gotest-work-")
			gotest.NoError(it, err)
			ctx.dirs = append(ctx.dirs, dir)
			gotest.NoError(it, os.WriteFile(filepath.Join(dir, ".pid"), []byte(strconv.Itoa(os.Getpid())), 0o600))

			gotestrunner.CleanStaleOverlays()

			_, err = os.Stat(dir)
			gotest.False(it, os.IsNotExist(err), "expected the live run's work dir to be kept")
		})

		w.It("ignores non-overlay directories", func(it *gotest.T) {
			dir, err := os.MkdirTemp(os.TempDir(), "unrelated-")
			gotest.NoError(it, err)
			ctx.dirs = append(ctx.dirs, dir)

			gotestrunner.CleanStaleOverlays()

			_, err = os.Stat(dir)
			gotest.False(it, os.IsNotExist(err), "expected non-overlay dir to be untouched")
		})
	})
}

func (s *OverlayTestSuite) TestCacheRoot(t *gotest.T, ctx *overlayDirsCtx) {
	t.When("cache root resolution", func(w *gotest.T) {
		w.It("respects GOTEST_CACHE_DIR env var", func(it *gotest.T) {
			dir := it.TempDir()
			it.Setenv(protocol.EnvCacheDir, dir)

			root, err := gotestrunner.ExportCacheRoot()
			gotest.NoError(it, err)
			gotest.Equal(it, dir, root)
		})

		w.It("falls back to UserCacheDir/gotest when env is unset", func(it *gotest.T) {
			it.Setenv(protocol.EnvCacheDir, "")

			root, err := gotestrunner.ExportCacheRoot()
			gotest.NoError(it, err)
			gotest.True(it, strings.HasSuffix(root, "gotest"))
			gotest.NotEmpty(it, root)
		})
	})

	t.When("an earlier CLI left overlay entries in the cache", func(w *gotest.T) {
		w.It("removes the entries older than 7 days and keeps the fresh ones", func(it *gotest.T) {
			cacheDir := it.TempDir()
			it.Setenv(protocol.EnvCacheDir, cacheDir)

			overlaysDir := filepath.Join(cacheDir, "overlays")
			_ = os.MkdirAll(overlaysDir, 0755)

			// Create an old entry.
			oldDir := filepath.Join(overlaysDir, "old-hash-entry")
			_ = os.MkdirAll(oldDir, 0755)
			_ = os.WriteFile(filepath.Join(oldDir, "overlay.json"), []byte("{}"), 0600)
			oldTime := time.Now().Add(-8 * 24 * time.Hour)
			_ = os.Chtimes(oldDir, oldTime, oldTime)

			// Create a fresh entry.
			freshDir := filepath.Join(overlaysDir, "fresh-hash-entry")
			_ = os.MkdirAll(freshDir, 0755)
			_ = os.WriteFile(filepath.Join(freshDir, "overlay.json"), []byte("{}"), 0600)

			gotestrunner.CleanStaleOverlays()

			_, err := os.Stat(oldDir)
			gotest.True(it, os.IsNotExist(err), "expected old cache entry to be removed")

			_, err = os.Stat(freshDir)
			gotest.False(it, os.IsNotExist(err), "expected fresh cache entry to be kept")
		})
	})
}

// Each run's overlay is its own: it lives in the run's work dir, so no
// other run can hand it out half-written or remove it.
func (s *OverlayTestSuite) TestTheOverlayIsTheRunsOwn(t *gotest.T, ctx *overlayDirsCtx) {
	cache := t.TempDir()
	t.Setenv(protocol.EnvCacheDir, cache)
	// open generates an overlay of nothing, as a run of no suite does.
	open := func(it *gotest.T, debug, noCache bool) (*gotestrunner.OverlayResult, func()) {
		overlay, cleanup, err := gotestrunner.GenerateOverlay(nil, nil, debug, noCache, false)
		gotest.NoError(it, err)
		ctx.dirs = append(ctx.dirs, overlay.WorkDir)
		return overlay, cleanup
	}

	t.When("two runs generate the same overlay", func(w *gotest.T) {
		w.It("gives each a dir of its own inside its work dir", func(it *gotest.T) {
			first, endFirst := open(it, false, false)
			second, endSecond := open(it, false, false)
			gotest.NotEqual(it, first.OverlayDir, second.OverlayDir)
			for _, o := range []*gotestrunner.OverlayResult{first, second} {
				gotest.Equal(it, filepath.Join(o.WorkDir, "overlay"), o.OverlayDir)
				gotest.Equal(it, "-overlay="+filepath.Join(o.OverlayDir, "overlay.json"), o.OverlayFlag)
				_, err := os.Stat(filepath.Join(o.OverlayDir, "overlay.json"))
				gotest.NoError(it, err)
			}
			endFirst()
			endSecond()
		})
		w.It("writes nothing under the cache's overlays dir", func(it *gotest.T) {
			_, end := open(it, false, false)
			end()
			_, err := os.Stat(filepath.Join(cache, "overlays"))
			gotest.True(it, os.IsNotExist(err), "a run wrote under %s", filepath.Join(cache, "overlays"))
		})
	})

	t.When("a run ends", func(w *gotest.T) {
		w.It("removes its overlay with its work dir", func(it *gotest.T) {
			overlay, end := open(it, false, false)
			end()
			_, err := os.Stat(overlay.OverlayDir)
			gotest.True(it, os.IsNotExist(err), "the overlay %s outlived its run", overlay.OverlayDir)
		})
	})

	t.When("a --debug run ends", func(w *gotest.T) {
		w.It("keeps its overlay and marks its work dir kept", func(it *gotest.T) {
			overlay, end := open(it, true, false)
			end()
			_, err := os.Stat(filepath.Join(overlay.OverlayDir, "overlay.json"))
			gotest.NoError(it, err)
			_, err = os.Stat(filepath.Join(overlay.WorkDir, ".keep"))
			gotest.NoError(it, err)
		})
	})

	t.When("a --no-cache run generates its overlay", func(w *gotest.T) {
		w.It("puts it in its work dir as any run does, with no binary cache", func(it *gotest.T) {
			overlay, end := open(it, false, true)
			gotest.Equal(it, filepath.Join(overlay.WorkDir, "overlay"), overlay.OverlayDir)
			gotest.Empty(it, overlay.BinaryCacheRoot)
			end()
		})
	})
}

// The work dir names the run that owns it, so a later run's sweep can tell
// a crashed run's leftover from one still in use.
func (s *OverlayTestSuite) TestWorkDirNamesItsRun(t *gotest.T, ctx *overlayDirsCtx) {
	t.Setenv(protocol.EnvCacheDir, t.TempDir())
	overlay, cleanup, err := gotestrunner.GenerateOverlay(nil, nil, false, true, false)
	gotest.NoError(t, err)
	pid, readErr := os.ReadFile(filepath.Join(overlay.WorkDir, ".pid"))
	cleanup()
	t.It("writes the run's pid into its work dir", func(it *gotest.T) {
		gotest.NoError(it, readErr)
		gotest.Equal(it, strconv.Itoa(os.Getpid()), string(pid))
	})
}

// A --debug run keeps its work dir and overlay for inspection: a later run's
// sweep leaves them, though their owner is gone.
func (s *OverlayTestSuite) TestADebugRunKeepsItsDirs(t *gotest.T, ctx *overlayDirsCtx) {
	t.Setenv(protocol.EnvCacheDir, t.TempDir())
	overlay, _, err := gotestrunner.GenerateOverlay(nil, nil, true, true, false)
	gotest.NoError(t, err)
	ctx.dirs = append(ctx.dirs, overlay.WorkDir)
	for _, dir := range ctx.dirs {
		// The debug run is over.
		gotest.NoError(t, os.WriteFile(filepath.Join(dir, ".pid"), []byte("999999999"), 0o600))
	}

	gotestrunner.CleanStaleOverlays()

	_, workErr := os.Stat(overlay.WorkDir)
	_, overlayErr := os.Stat(overlay.OverlayDir)
	t.It("a later run's sweep keeps its work dir and its overlay", func(it *gotest.T) {
		gotest.NoError(it, workErr)
		gotest.NoError(it, overlayErr)
	})
}

// A sweep tells a dir's owner by more than its pid: the boot and the pid
// namespace it was read in. Only a dir from this boot and namespace whose
// pid runs no process is removed; every other stamp keeps it.
func (s *OverlayTestSuite) TestASweepTellsAnOwnerByMoreThanItsPid(t *gotest.T, ctx *overlayDirsCtx) {
	// stamp makes a work dir and an overlay dir as a run does, its identity
	// read as boot and ns.
	stamp := func(it *gotest.T, boot, ns string) []string {
		ctx.restore = gotestrunner.ExportStubOwnerIdentity(func() string { return ns }, func() string { return boot })
		it.Setenv(protocol.EnvCacheDir, it.TempDir())
		overlay, _, err := gotestrunner.GenerateOverlay(nil, nil, false, true, false)
		gotest.NoError(it, err)
		ctx.restore()
		ctx.dirs = append(ctx.dirs, overlay.WorkDir)
		return []string{overlay.WorkDir}
	}
	// sweep sweeps with this process's identity read as boot and ns.
	sweep := func(boot, ns string) {
		ctx.restore = gotestrunner.ExportStubOwnerIdentity(func() string { return ns }, func() string { return boot })
		gotestrunner.CleanStaleOverlays()
		ctx.restore()
	}
	// age makes dirs older than 7 days.
	age := func(it *gotest.T, dirs []string) {
		then := time.Now().Add(-8 * 24 * time.Hour)
		for _, dir := range dirs {
			gotest.NoError(it, os.Chtimes(dir, then, then))
		}
	}
	kept := func(it *gotest.T, dirs []string) {
		for _, dir := range dirs {
			_, err := os.Stat(dir)
			gotest.NoError(it, err, "expected %s to be kept", dir)
		}
	}
	removed := func(it *gotest.T, dirs []string) {
		for _, dir := range dirs {
			_, err := os.Stat(dir)
			gotest.True(it, os.IsNotExist(err), "expected %s to be removed", dir)
		}
	}
	// deadHere makes dirs name a pid no process has here.
	deadHere := func(it *gotest.T, dirs []string) {
		for _, dir := range dirs {
			gotest.NoError(it, os.WriteFile(filepath.Join(dir, ".pid"), []byte("999999999"), 0o600))
		}
	}
	// keptWhenDead checks that dirs stamped as boot and ns, their pid dead
	// here, outlive a sweep read as selfBoot and selfNS, at any age.
	keptWhenDead := func(w *gotest.T, boot, ns, selfBoot, selfNS string) {
		w.It("keeps them, though their pid names no process here", func(it *gotest.T) {
			dirs := stamp(it, boot, ns)
			deadHere(it, dirs)

			sweep(selfBoot, selfNS)

			kept(it, dirs)
		})
		w.It("keeps them once they are older than 7 days", func(it *gotest.T) {
			dirs := stamp(it, boot, ns)
			deadHere(it, dirs)
			age(it, dirs)

			sweep(selfBoot, selfNS)

			kept(it, dirs)
		})
	}

	t.When("a run in another boot, or on another host, stamped the dirs", func(w *gotest.T) {
		keptWhenDead(w, "boot-a", "pid:[1]", "boot-b", "pid:[1]")
	})

	t.When("a run in another pid namespace stamped the dirs", func(w *gotest.T) {
		keptWhenDead(w, "boot-a", "pid:[1]", "boot-a", "pid:[2]")
	})

	t.When("neither the run that stamped the dirs nor this one could read its boot", func(w *gotest.T) {
		keptWhenDead(w, "unknown", "pid:[1]", "unknown", "pid:[1]")
	})

	t.When("the run that stamped the dirs could read its boot and this one cannot", func(w *gotest.T) {
		keptWhenDead(w, "boot-a", "pid:[1]", "unknown", "pid:[1]")
	})

	t.When("neither the run that stamped the dirs nor this one could read its pid namespace", func(w *gotest.T) {
		keptWhenDead(w, "boot-a", "unknown", "boot-a", "unknown")
	})

	t.When("the run that stamped the dirs could read its pid namespace and this one cannot", func(w *gotest.T) {
		keptWhenDead(w, "boot-a", "pid:[1]", "boot-a", "unknown")
	})

	t.When("the dirs' identity file is there but cannot be read", func(w *gotest.T) {
		w.It("keeps them, though their pid names no process here", func(it *gotest.T) {
			if runtime.GOOS == "windows" || os.Geteuid() == 0 {
				it.Skipf("file modes do not stop this reader")
			}
			dirs := stamp(it, "boot-a", "pid:[1]")
			deadHere(it, dirs)
			for _, dir := range dirs {
				gotest.NoError(it, os.Chmod(filepath.Join(dir, ".owner"), 0))
			}

			sweep("boot-a", "pid:[1]")

			kept(it, dirs)
		})
	})

	t.When("a run in this boot and pid namespace stamped the dirs and their pid now names another live process", func(w *gotest.T) {
		w.It("keeps them", func(it *gotest.T) {
			dirs := stamp(it, "boot-a", "pid:[1]")
			for _, dir := range dirs {
				gotest.NoError(it, os.WriteFile(filepath.Join(dir, ".pid"), []byte(strconv.Itoa(os.Getppid())), 0o600))
			}
			age(it, dirs)

			sweep("boot-a", "pid:[1]")

			kept(it, dirs)
		})
	})

	t.When("a run in this boot and pid namespace stamped the dirs and its pid runs no process", func(w *gotest.T) {
		w.It("removes them", func(it *gotest.T) {
			dirs := stamp(it, "boot-a", "pid:[1]")
			deadHere(it, dirs)

			sweep("boot-a", "pid:[1]")

			removed(it, dirs)
		})

		w.It("removes them as this platform's own readers tell it", func(it *gotest.T) {
			it.Setenv(protocol.EnvCacheDir, it.TempDir())
			overlay, _, err := gotestrunner.GenerateOverlay(nil, nil, false, true, false)
			gotest.NoError(it, err)
			dirs := []string{overlay.WorkDir}
			ctx.dirs = append(ctx.dirs, dirs...)
			deadHere(it, dirs)

			gotestrunner.CleanStaleOverlays()

			removed(it, dirs)
		})
	})

	t.When("a run in this boot and pid namespace stamped the dirs and its pid runs", func(w *gotest.T) {
		w.It("keeps them, though the pid may now name another process", func(it *gotest.T) {
			dirs := stamp(it, "boot-a", "pid:[1]")
			age(it, dirs)

			sweep("boot-a", "pid:[1]")

			kept(it, dirs)
		})

		w.It("keeps them as this platform's own readers tell it", func(it *gotest.T) {
			it.Setenv(protocol.EnvCacheDir, it.TempDir())
			overlay, _, err := gotestrunner.GenerateOverlay(nil, nil, false, true, false)
			gotest.NoError(it, err)
			ctx.dirs = append(ctx.dirs, overlay.WorkDir)

			gotestrunner.CleanStaleOverlays()

			kept(it, []string{overlay.WorkDir, overlay.OverlayDir})
		})
	})
}
