package testkit_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// deadPID is above every platform's pid ceiling, so no process holds it.
const deadPID = "999999999"

// CLIDirTestSuite: building the CLI sweeps what gone processes left, and a
// CLI built once from the same sources is reused, not built again.
// Sequential: the methods move the process's temp dir.
type CLIDirTestSuite struct{}

func (s *CLIDirTestSuite) TestStaleCLIDirsAreSwept(t *gotest.T) {
	tmp := privateTempDir(t)
	aged := time.Now().Add(-48 * time.Hour) // past every age the sweep allows
	dead := cliDir(t, tmp, "gotest-cli-dead", ".pid", deadPID)
	live := cliDir(t, tmp, "gotest-cli-live", ".pid", strconv.Itoa(os.Getpid()))
	fresh := cliDir(t, tmp, "gotest-cli-fresh", "", "")
	old := agedPath(t, cliDir(t, tmp, "gotest-cli-old", "", ""), aged)
	idleBuild := file(t, tmp, testkit.ExportBuildPrefix+"idle", "cli")
	staleBuild := agedPath(t, file(t, tmp, testkit.ExportBuildPrefix+"stale", "cli"), aged)
	staleUse := agedPath(t, file(t, tmp, testkit.ExportUsePrefix+"stale-"+deadPID, deadPID), aged)
	usedBuild := agedPath(t, file(t, tmp, testkit.ExportBuildPrefix+"used", "cli"), aged)
	freshUse := file(t, tmp, testkit.ExportUsePrefix+"used-"+deadPID, deadPID)
	deadMarker := filepath.Dir(plantMarker(t, tmp, "dead", deadPID))
	liveMarker := filepath.Dir(plantMarker(t, tmp, "live", strconv.Itoa(os.Getpid())))
	hungMarker := filepath.Dir(agedPath(t, plantMarker(t, tmp, "hung", strconv.Itoa(os.Getpid())), aged))
	oldUnknown := agedPath(t, file(t, tmp, "gotest-cli-unknown", ""), aged)

	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	binary, err := testkit.ExportResolveCLI(t.Context(), root)
	gotest.NoError(t, err)

	t.When("testkit builds its CLI", func(w *gotest.T) {
		w.It("removes a dir whose process is gone", func(it *gotest.T) {
			gotest.False(it, exists(dead))
		})
		w.It("keeps a dir whose process is alive", func(it *gotest.T) {
			gotest.True(it, exists(live))
		})
		w.It("removes a pid-less dir once it is old enough", func(it *gotest.T) {
			gotest.False(it, exists(old))
		})
		w.It("keeps a pid-less dir too new to have been stamped", func(it *gotest.T) {
			gotest.True(it, exists(fresh))
		})
		w.It("removes a published build nobody used for a while, and its use records", func(it *gotest.T) {
			gotest.False(it, exists(staleBuild))
			gotest.False(it, exists(staleUse))
		})
		w.It("keeps a published build published moments ago, for the next process", func(it *gotest.T) {
			gotest.True(it, exists(idleBuild))
		})
		w.It("keeps an old published build used moments ago", func(it *gotest.T) {
			gotest.True(it, exists(usedBuild))
			gotest.True(it, exists(freshUse))
		})
		w.It("removes the build marker of a builder that is gone or hung", func(it *gotest.T) {
			gotest.False(it, exists(deadMarker))
			gotest.False(it, exists(hungMarker))
		})
		w.It("keeps the build marker of a live builder", func(it *gotest.T) {
			gotest.True(it, exists(liveMarker))
		})
		w.It("removes a file it does not know once it is old enough", func(it *gotest.T) {
			gotest.False(it, exists(oldUnknown))
		})
		w.It("copies its CLI into a dir of its own, stamped with its pid", func(it *gotest.T) {
			gotest.Regexp(it, "^gotest-cli-[0-9]+$", filepath.Base(filepath.Dir(binary)))
			gotest.True(it, exists(binary))
			gotest.True(it, testkit.ExportCLIDirAlive(filepath.Dir(binary)))
		})
		w.It("publishes the build and leaves no dir it built in", func(it *gotest.T) {
			gotest.Len(it, glob(it, tmp, testkit.ExportBuildPrefix+"*"), 3)
			gotest.ElementsMatch(it, []string{live, fresh, liveMarker, filepath.Dir(binary)}, cliSubdirs(it, tmp))
		})
	})
}

func (s *CLIDirTestSuite) TestABuiltCLIIsReused(t *gotest.T) {
	tmp := privateTempDir(t)
	var builds atomic.Int32
	defer testkit.ExportFakeBuilds(testkit.ExportCLIKey, func(ctx context.Context, root, dir string) (string, error) {
		builds.Add(1)
		return testkit.BuildCLI(ctx, root, dir)
	})()
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	first, err := testkit.ExportResolveCLI(t.Context(), root)
	gotest.NoError(t, err)
	second, err := testkit.ExportResolveCLI(t.Context(), root)
	gotest.NoError(t, err)

	t.When("the sources are unchanged", func(w *gotest.T) {
		w.It("builds once and publishes one build", func(it *gotest.T) {
			gotest.Equal(it, int32(1), builds.Load())
			gotest.Len(it, glob(it, tmp, testkit.ExportBuildPrefix+"*"), 1)
		})
		w.It("hands each caller a copy of its own of the same binary", func(it *gotest.T) {
			gotest.NotEqual(it, filepath.Dir(first), filepath.Dir(second))
			gotest.Equal(it, readFile(it, first), readFile(it, second))
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

func (s *CLIDirTestSuite) TestSweepsRacingClaimsNeverCostAUserItsBinary(t *gotest.T) {
	r := raceClaimsAndSweeps(t)
	t.When("sweeps race claims and builds of the same build", func(w *gotest.T) {
		w.It("serves claims, reuses the published build, and sweeps take it between claims", func(it *gotest.T) {
			gotest.Greater(it, r.claims, r.builds)
			gotest.Greater(it, r.builds, int64(1))
		})
		w.It("builds one at a time", func(it *gotest.T) {
			gotest.Zero(it, r.overlaps)
		})
		w.It("fails no claim", func(it *gotest.T) {
			gotest.Zero(it, r.failed, "%d of %d claims failed, first: %v", r.failed, r.claims, r.firstErr)
		})
		w.It("never hands out part of a build", func(it *gotest.T) {
			gotest.Zero(it, r.partial, "%d of %d claims got part of a build", r.partial, r.claims)
		})
		w.It("never takes a binary from a live user", func(it *gotest.T) {
			gotest.Zero(it, r.vanished, "%d of %d claims lost their binary", r.vanished, r.claims)
		})
	})
}

func (s *CLIDirTestSuite) TestABuildMarkerHoldsOthersOffOnlyWhileLive(t *gotest.T) {
	for t, row := range gotest.Each(t, []struct {
		Desc   string
		pid    string
		age    time.Duration
		builds int32
	}{
		{"a live builder's marker", "live", 0, 0},
		{"a dead builder's marker", deadPID, 0, 1},
		{"a live builder's marker past the wait", "live", 48 * time.Hour, 1},
		{"an empty marker, left by a release", "", 0, 1},
	}) {
		tmp := t.TempDir()
		pid := row.pid
		if pid == "live" {
			pid = strconv.Itoa(os.Getpid())
		}
		stamp := filepath.Join(tmp, testkit.ExportBuildingPrefix+"k")
		if pid == "" {
			gotest.NoError(t, os.Mkdir(stamp, 0o750))
		} else {
			stamp = plantMarker(t, tmp, "k", pid)
		}
		if row.age > 0 {
			agedPath(t, stamp, time.Now().Add(-row.age))
		}
		var builds atomic.Int32
		build := func(_ context.Context, dir string) (string, error) {
			builds.Add(1)
			binary := filepath.Join(dir, testkit.ExportCLIName())
			return binary, os.WriteFile(binary, []byte("built"), 0o600)
		}
		private := t.TempDir()
		store := testkit.NewExportStore(tmp, 1, func(p int) bool { return p == os.Getpid() }, time.Hour)
		var binary string
		var err error
		// Far short of the wait.
		ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
		defer cancel()
		wait := gotest.Go(t, func() { binary, err = store.Get(ctx, "k", private, build) })
		// A process that does not wait builds at once.
		time.Sleep(200 * time.Millisecond)
		if row.builds == 0 {
			gotest.Zero(t, builds.Load(), "built while a live builder held the marker")
			staged := file(t, t.TempDir(), "staged", "published")
			gotest.NoError(t, os.Rename(staged, filepath.Join(tmp, testkit.ExportBuildPrefix+"k")))
		}
		wait()
		t.It("builds only when the marker holds nobody off", func(it *gotest.T) {
			gotest.NoError(it, err)
			gotest.Equal(it, row.builds, builds.Load())
			want := "published"
			if row.builds > 0 {
				want = "built"
			}
			gotest.Equal(it, want, string(readFile(it, binary)))
		})
	}
}

func (s *CLIDirTestSuite) TestABuilderHoldsTheMarkerHoweverOftenItIsContended(t *gotest.T) {
	tmp := t.TempDir()
	// Each try to create the marker first finds one a dead builder left.
	const contended = 8
	var tries int
	defer testkit.ExportBeforeMark(func(marker string) {
		if tries++; tries <= contended {
			_, _ = testkit.ExportPlantMarker(filepath.Dir(marker), "k", deadPID)
		}
	})()
	var held []string
	build := func(_ context.Context, dir string) (string, error) {
		held = append(held, markerPIDs(filepath.Join(tmp, testkit.ExportBuildingPrefix+"k"))...)
		binary := filepath.Join(dir, testkit.ExportCLIName())
		return binary, os.WriteFile(binary, []byte("built"), 0o600)
	}
	store := testkit.NewExportStore(tmp, 7, func(p int) bool { return p == 7 }, time.Hour)
	_, err := store.Get(t.Context(), "k", t.TempDir(), build)

	t.When("dead builders' markers stand in its way try after try", func(w *gotest.T) {
		w.It("builds once, holding the marker while it builds", func(it *gotest.T) {
			gotest.NoError(it, err)
			gotest.Equal(it, []string{"7"}, held)
		})
		w.It("tried until the contention ended", func(it *gotest.T) {
			gotest.Greater(it, tries, contended)
		})
	})
}

func (s *CLIDirTestSuite) TestABuildMarkerNeverShowsWithoutItsPid(t *gotest.T) {
	tmp := t.TempDir()
	marker := filepath.Join(tmp, testkit.ExportBuildingPrefix+"k")
	published := filepath.Join(tmp, testkit.ExportBuildPrefix+"k")
	// Counts up through each round: 0 mod 3 while a builder marks, 1 once it
	// builds, 2 once it is done.
	var phase atomic.Int32
	phase.Store(2)
	build := func(_ context.Context, dir string) (string, error) {
		phase.Add(1)
		binary := filepath.Join(dir, testkit.ExportCLIName())
		return binary, os.WriteFile(binary, []byte("built"), 0o600)
	}
	store := testkit.NewExportStore(tmp, 7, func(p int) bool { return p == 7 }, time.Hour)
	// An empty marker holds nobody off, so one must never show before its
	// builder's stamp is in it.
	var done atomic.Bool
	var reads, empty atomic.Int64
	wait := gotest.Go(t, func() {
		for !done.Load() {
			before := phase.Load()
			entries, err := os.ReadDir(marker)
			if err == nil && before%3 == 0 && phase.Load() == before {
				reads.Add(1)
				if len(entries) == 0 {
					empty.Add(1)
				}
			}
		}
	})
	var err error
	for range 300 {
		phase.Add(1)
		if _, err = store.Get(t.Context(), "k", t.TempDir(), build); err != nil {
			break
		}
		phase.Add(1)
		_ = os.Remove(published)
		// Windows refuses a release while the reader holds the marker open.
		for exists(marker) {
			_ = os.RemoveAll(marker)
		}
	}
	done.Store(true)
	wait()

	t.When("a builder marks its build while another process reads the marker", func(w *gotest.T) {
		w.It("shows the marker only with the builder's stamp in it", func(it *gotest.T) {
			gotest.NoError(it, err)
			gotest.Greater(it, reads.Load(), int64(0))
			gotest.Zero(it, empty.Load(), "%d of %d reads found the marker empty", empty.Load(), reads.Load())
		})
	})
}

func (s *CLIDirTestSuite) TestAReleaseHeldUpByAReaderStillFreesTheMarker(t *gotest.T) {
	tmp := t.TempDir()
	marker := filepath.Join(tmp, testkit.ExportBuildingPrefix+"k")
	var builds int
	var reader *os.File
	var opened error
	build := func(_ context.Context, dir string) (string, error) {
		if builds++; builds == 1 {
			// Held open through the release, which Windows refuses a removal for.
			reader, opened = os.Open(marker)
		}
		binary := filepath.Join(dir, testkit.ExportCLIName())
		return binary, os.WriteFile(binary, []byte("built"), 0o600)
	}
	store := testkit.NewExportStore(tmp, 7, func(p int) bool { return p == 7 }, time.Hour)
	_, first := store.Get(t.Context(), "k", t.TempDir(), build)
	gotest.NoError(t, opened)
	gotest.NoError(t, reader.Close())
	gotest.NoError(t, os.Remove(filepath.Join(tmp, testkit.ExportBuildPrefix+"k")))
	// Far short of the wait a marker of its own would hold it off for.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	_, second := store.Get(ctx, "k", t.TempDir(), build)

	t.When("a reader held the marker open while its builder released it", func(w *gotest.T) {
		w.It("builds again at once when nothing is published", func(it *gotest.T) {
			gotest.NoError(it, first)
			gotest.NoError(it, second)
			gotest.Equal(it, 2, builds)
		})
	})
}

func (s *CLIDirTestSuite) TestATakeOverNeverRemovesAMarkerPutInItsPlace(t *gotest.T) {
	tmp := t.TempDir()
	_, err := testkit.ExportPlantMarker(tmp, "k", deadPID)
	gotest.NoError(t, err)
	var fresh string
	var takes int
	defer testkit.ExportBeforeTake(func(marker string) {
		if takes++; takes == 1 {
			// Another taker clears the dead builder's marker, and a new
			// builder marks.
			gotest.NoError(t, os.RemoveAll(marker))
			fresh, err = testkit.ExportPlantMarker(tmp, "k", strconv.Itoa(os.Getpid()))
		}
	})()
	testkit.NewExportStore(tmp, 9, func(p int) bool { return p == os.Getpid() }, time.Hour).Sweep()

	t.When("a new builder marks between a take-over's look at a dead marker and its removal", func(w *gotest.T) {
		w.It("keeps the new builder's marker", func(it *gotest.T) {
			gotest.NoError(it, err)
			gotest.Equal(it, 1, takes)
			gotest.True(it, exists(fresh))
		})
	})
}

func (s *CLIDirTestSuite) TestAReleaseNeverRemovesAMarkerTakenOverFromIt(t *gotest.T) {
	tmp := t.TempDir()
	marker := filepath.Join(tmp, testkit.ExportBuildingPrefix+"k")
	alive := func(p int) bool { return p == 7 || p == 8 }
	var fresh string
	var planted error
	var builds atomic.Int32
	build := func(_ context.Context, dir string) (string, error) {
		if builds.Add(1) == 1 {
			// Taken for hung, its marker goes to builder 8.
			planted = os.RemoveAll(marker)
			if planted == nil {
				fresh, planted = testkit.ExportPlantMarker(tmp, "k", "8")
			}
		}
		binary := filepath.Join(dir, testkit.ExportCLIName())
		return binary, os.WriteFile(binary, []byte("built"), 0o600)
	}
	_, first := testkit.NewExportStore(tmp, 7, alive, time.Hour).Get(t.Context(), "k", t.TempDir(), build)
	gotest.NoError(t, os.Remove(filepath.Join(tmp, testkit.ExportBuildPrefix+"k")))
	// Long enough to build at once when nothing holds it off.
	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancel()
	_, second := testkit.NewExportStore(tmp, 9, alive, time.Hour).Get(ctx, "k", t.TempDir(), build)

	t.When("a builder releases a marker another builder took over from it", func(w *gotest.T) {
		w.It("keeps the new builder's marker", func(it *gotest.T) {
			gotest.NoError(it, first)
			gotest.NoError(it, planted)
			gotest.True(it, exists(fresh))
		})
		w.It("still holds others off with it", func(it *gotest.T) {
			gotest.ErrorIs(it, second, context.DeadlineExceeded)
			gotest.Equal(it, int32(1), builds.Load())
		})
	})
}

func (s *CLIDirTestSuite) TestAMarkerRenameRefusedForAMomentIsTriedAgain(t *gotest.T) {
	held := errors.New("held open")
	for t, row := range gotest.Each(t, []struct {
		Desc    string
		refused int
		builds  int32
	}{
		{"refused a few times", 3, 1},
		{"refused every time", 1 << 30, 0},
	}) {
		var renames int
		restore := testkit.ExportRenameMarker(func(from, to string) error {
			if renames++; renames <= row.refused {
				return held
			}
			return os.Rename(from, to)
		})
		var builds atomic.Int32
		build := func(_ context.Context, dir string) (string, error) {
			builds.Add(1)
			binary := filepath.Join(dir, testkit.ExportCLIName())
			return binary, os.WriteFile(binary, []byte("built"), 0o600)
		}
		_, err := testkit.NewExportStore(t.TempDir(), 7, func(p int) bool { return p == 7 }, time.Hour).Get(t.Context(), "k", t.TempDir(), build)
		restore()
		t.It("builds once the rename goes through, and gives up after a few tries", func(it *gotest.T) {
			gotest.Equal(it, row.builds, builds.Load())
			if row.builds > 0 {
				gotest.NoError(it, err)
				gotest.Greater(it, renames, row.refused)
			} else {
				gotest.ErrorIs(it, err, held)
				gotest.Less(it, renames, 10)
			}
		})
	}
}

type raceCounts struct {
	claims, builds, overlaps, failed, partial, vanished int64
	firstErr                                            error
}

// raceClaimsAndSweeps runs claimers and sweepers of one build for two
// seconds. Each claimer is a fresh process that builds when nothing is
// published, keeps its copy a moment, and exits.
func raceClaimsAndSweeps(t *gotest.T) raceCounts {
	tmp := t.TempDir()
	// Fake pids, far above any real one, live while their goroutine says so.
	var live sync.Map
	alive := func(pid int) bool { _, ok := live.Load(pid); return ok }
	var nextPID atomic.Int64
	nextPID.Store(2_000_000_000)
	pid := func() int { p := int(nextPID.Add(1)); live.Store(p, true); return p }
	full := bytes.Repeat([]byte("0123456789abcdef"), 4096)

	var stop atomic.Bool
	var firstErr atomic.Value
	var active atomic.Int32
	var n struct{ claims, builds, overlaps, failed, partial, vanished atomic.Int64 }
	// build writes in pieces, as a linker does.
	build := func(_ context.Context, dir string) (string, error) {
		n.builds.Add(1)
		if active.Add(1) > 1 {
			n.overlaps.Add(1)
		}
		defer active.Add(-1)
		binary := filepath.Join(dir, testkit.ExportCLIName())
		f, err := os.Create(binary) //nolint:gosec // G304: the test's own store
		if err != nil {
			return "", err
		}
		defer f.Close()
		for i := 0; i < len(full); i += 4096 {
			if _, err := f.Write(full[i : i+4096]); err != nil {
				return "", err
			}
			time.Sleep(time.Millisecond)
		}
		return binary, nil
	}
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for !stop.Load() {
				testkit.NewExportStore(tmp, 1, alive, 0).Sweep()
			}
		})
	}
	for range 8 {
		wg.Go(func() {
			for !stop.Load() {
				me := pid()
				private, err := testkit.ExportNewStampedDir(tmp, me)
				var binary string
				if err == nil {
					binary, err = testkit.NewExportStore(tmp, me, alive, 0).Get(t.Context(), "stress", private, build)
				}
				n.claims.Add(1)
				switch {
				case err != nil:
					n.failed.Add(1)
					firstErr.CompareAndSwap(nil, err)
				case !bytes.Equal(full, readOrNil(binary)):
					n.partial.Add(1)
				default:
					for range 50 {
						if !exists(binary) {
							n.vanished.Add(1)
							break
						}
					}
				}
				live.Delete(me)
				time.Sleep(5 * time.Millisecond)
			}
		})
	}
	time.Sleep(2 * time.Second)
	stop.Store(true)
	wg.Wait()
	r := raceCounts{n.claims.Load(), n.builds.Load(), n.overlaps.Load(), n.failed.Load(), n.partial.Load(), n.vanished.Load(), nil}
	if err, ok := firstErr.Load().(error); ok {
		r.firstErr = err
	}
	return r
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

// plantMarker puts key's build marker in dir, held by pid, and returns its
// stamp.
func plantMarker(t *gotest.T, dir, key, pid string) string {
	stamp, err := testkit.ExportPlantMarker(dir, key, pid)
	gotest.NoError(t, err)
	return stamp
}

// markerPIDs lists the pids whose stamps the marker holds.
func markerPIDs(marker string) []string {
	entries, _ := os.ReadDir(marker)
	var pids []string
	for _, e := range entries {
		pids = append(pids, strings.SplitN(e.Name(), "-", 2)[0])
	}
	return pids
}

// file writes data to dir/name and returns its path.
func file(t *gotest.T, dir, name, data string) string {
	path := filepath.Join(dir, name)
	write(t, path, data)
	return path
}

// agedPath dates path back to at.
func agedPath(t *gotest.T, path string, at time.Time) string {
	gotest.NoError(t, os.Chtimes(path, at, at))
	return path
}

func glob(t *gotest.T, dir, pattern string) []string {
	paths, err := filepath.Glob(filepath.Join(dir, pattern))
	gotest.NoError(t, err)
	return paths
}

// cliSubdirs lists the CLI dirs in tmp.
func cliSubdirs(t *gotest.T, tmp string) []string {
	var dirs []string
	for _, path := range glob(t, tmp, "gotest-cli-*") {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			dirs = append(dirs, path)
		}
	}
	return dirs
}

func readFile(t *gotest.T, path string) []byte {
	data, err := os.ReadFile(path) //nolint:gosec // G304: a file the test made
	gotest.NoError(t, err)
	return data
}

func readOrNil(path string) []byte {
	data, _ := os.ReadFile(path) //nolint:gosec // G304: a file the test made
	return data
}

func write(t *gotest.T, path, data string) {
	gotest.NoError(t, os.WriteFile(path, []byte(data), 0o600))
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
