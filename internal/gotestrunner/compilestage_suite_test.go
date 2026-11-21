package gotestrunner_test

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CompileStageTestSuite: a run places its cached binaries into a stage dir
// of its own, judges each slot by what go did to it, and publishes what go
// rebuilt into the cache by a rename.
type CompileStageTestSuite struct{}

func (s *CompileStageTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

type stageCtx struct {
	cache, stage string
}

func (s *CompileStageTestSuite) BeforeEach(t *gotest.T) *stageCtx {
	ctx := &stageCtx{cache: t.TempDir(), stage: filepath.Join(t.TempDir(), "stage")}
	entry := filepath.Join(ctx.cache, gotestrunner.ExportSanitizePkgName("m/a")+".test")
	gotest.NoError(t, os.WriteFile(entry, []byte("v1"), 0o600))
	// The record a run wrote when it last saw the entry linked.
	gotest.NoError(t, gotestrunner.ExportWriteSum(entry, gotest.Must(gotestrunner.ExportSumFile(entry))))
	return ctx
}

func (s *CompileStageTestSuite) TestReadiness(t *gotest.T, ctx *stageCtx) {
	dir := t.TempDir()
	file := func(name string) os.FileInfo {
		p := filepath.Join(dir, name)
		gotest.NoError(t, os.WriteFile(p, []byte(name), 0o600))
		return gotest.Must(os.Stat(p))
	}
	placed := file("placed")
	later := time.Now().Add(2 * time.Second)
	gotest.NoError(t, os.Chtimes(filepath.Join(dir, "placed"), later, later))
	touched := gotest.Must(os.Stat(filepath.Join(dir, "placed")))
	other := file("other")
	const running = -1
	type row struct {
		Desc        string
		Placed, Now os.FileInfo
		RenameSafe  bool
		Exited      int
		Want        gotestrunner.ExportReadiness
	}
	for t, r := range gotest.Each(t, []row{
		{Desc: "no file, go running", Placed: placed, Exited: running, Want: gotestrunner.ExportNotReady},
		{Desc: "no file, go done", Placed: placed, Exited: 0, Want: gotestrunner.ExportNotReady},
		// Another run's go touches the entry every slot links: no sign.
		{Desc: "same file, newer, go running", Placed: placed, Now: touched, Exited: running, Want: gotestrunner.ExportNotReady},
		{Desc: "same file, newer, exit 1", Placed: placed, Now: touched, Exited: 1, Want: gotestrunner.ExportNotReady},
		{Desc: "same file, newer, exit 0", Placed: placed, Now: touched, Exited: 0, Want: gotestrunner.ExportConfirmed},
		{Desc: "same file, not newer, go running", Placed: placed, Now: placed, Exited: running, Want: gotestrunner.ExportNotReady},
		{Desc: "same file, not newer, exit 1", Placed: placed, Now: placed, Exited: 1, Want: gotestrunner.ExportNotReady},
		{Desc: "same file, not newer, exit 0", Placed: placed, Now: placed, Exited: 0, Want: gotestrunner.ExportConfirmed},
		{Desc: "a new file where go renames", Placed: placed, Now: other, RenameSafe: true, Exited: running, Want: gotestrunner.ExportRebuilt},
		{Desc: "a new file in an empty slot where go renames", Now: other, RenameSafe: true, Exited: running, Want: gotestrunner.ExportRebuilt},
		{Desc: "a new file where go copies, go running", Placed: placed, Now: other, Exited: running, Want: gotestrunner.ExportNotReady},
		{Desc: "a new file in an empty slot where go copies, go running", Now: other, Exited: running, Want: gotestrunner.ExportNotReady},
		{Desc: "a new file where go copies, exit 0", Placed: placed, Now: other, Exited: 0, Want: gotestrunner.ExportRebuilt},
		{Desc: "a new file where go copies, exit 1", Now: other, Exited: 1, Want: gotestrunner.ExportRebuilt},
	}) {
		gotest.Equal(t, r.Want, gotestrunner.ExportBinaryReady(r.Placed, r.Now, r.RenameSafe, r.Exited))
	}
}

func (s *CompileStageTestSuite) TestRenameSafe(t *gotest.T, ctx *stageCtx) {
	dir := gotest.Must(os.Stat(t.TempDir()))
	t.It("holds where go renames its output into place", func(it *gotest.T) {
		gotest.True(it, gotestrunner.ExportRenameSafe("linux", dir, "/w/gotest-work-1/gotmp", "/home/u/.cache/go-build"))
	})
	t.It("does not hold where go copies", func(it *gotest.T) {
		gotest.False(it, gotestrunner.ExportRenameSafe("windows", dir, `C:\w\gotmp`, `C:\cache`))
		gotest.False(it, gotestrunner.ExportRenameSafe("linux", dir, "/tmp/gotest-work-1/gotmp", "/tmp/go"))
		gotest.False(it, gotestrunner.ExportRenameSafe("linux", dir, "/w/gotmp", ""))
	})
	t.It("does not hold in a setgid dir", func(it *gotest.T) {
		if runtime.GOOS == "windows" {
			it.Skipf("no setgid bit")
		}
		d := it.TempDir()
		gotest.NoError(it, os.Chmod(d, 0o755|os.ModeSetgid))
		gotest.False(it, gotestrunner.ExportRenameSafe("linux", gotest.Must(os.Stat(d)), "/w/gotmp", "/home/u/.cache/go-build"))
	})
}

func (s *CompileStageTestSuite) TestStageAndPublish(t *gotest.T, ctx *stageCtx) {
	t.When("a group is staged with a cache", func(w *gotest.T) {
		staged, err := gotestrunner.ExportStageGroup([]string{"m/a", "m/b"}, ctx.stage, ctx.cache)
		gotest.NoError(w, err)
		w.It("places the cached binary under go's name", func(it *gotest.T) {
			got, err := os.ReadFile(filepath.Join(ctx.stage, gotestrunner.ExportTestBinaryName("m/a", runtime.GOOS)))
			gotest.NoError(it, err)
			gotest.Equal(it, "v1", string(got))
			gotest.True(it, gotestrunner.ExportPlaced(&staged[0]))
		})
		w.It("leaves the slot of a package without an entry empty", func(it *gotest.T) {
			_, err := os.Stat(staged[1].Path)
			gotest.True(it, os.IsNotExist(err))
			gotest.False(it, gotestrunner.ExportPlaced(&staged[1]))
		})
		w.It("holds no lock once it returns", func(it *gotest.T) {
			unlock, err := gotestrunner.ExportLockFile(staged[0].Cached + ".lock")
			gotest.NoError(it, err)
			unlock()
		})
	})
	t.When("a rebuilt binary is published", func(w *gotest.T) {
		stage := filepath.Join(ctx.stage, "p")
		staged, err := gotestrunner.ExportStageGroup([]string{"m/a"}, stage, ctx.cache)
		gotest.NoError(w, err)
		held := gotest.Must(os.Open(staged[0].Cached)) // a run still reading the old entry
		gotest.NoError(w, os.Remove(staged[0].Path))
		gotest.NoError(w, os.WriteFile(staged[0].Path, []byte("v2"), 0o600))
		gotestrunner.ExportPublish(&staged[0], gotestrunner.ExportRebuilt)
		w.It("the cache entry holds the new bytes", func(it *gotest.T) {
			got, _ := os.ReadFile(staged[0].Cached)
			if runtime.GOOS == "windows" {
				// The entry is open: the publish is skipped, and done once it is not.
				gotest.Equal(it, "v1", string(got))
				return
			}
			gotest.Equal(it, "v2", string(got))
		})
		w.It("a reader of the old entry still reads the old bytes", func(it *gotest.T) {
			got, _ := io.ReadAll(held)
			gotest.Equal(it, "v1", string(got))
		})
		gotest.NoError(w, held.Close())
		if runtime.GOOS == "windows" {
			gotestrunner.ExportPublish(&staged[0], gotestrunner.ExportRebuilt)
			w.It("publishes once the entry is no longer open", func(it *gotest.T) {
				got, _ := os.ReadFile(staged[0].Cached)
				gotest.Equal(it, "v2", string(got))
			})
		}
		w.It("records the new entry's sum", func(it *gotest.T) {
			record, ok := gotestrunner.ExportReadSum(staged[0].Cached)
			gotest.True(it, ok)
			gotest.Equal(it, gotest.Must(gotestrunner.ExportSumFile(staged[0].Cached)), record)
		})
		w.It("leaves no temporary file in the cache", func(it *gotest.T) {
			entries, _ := os.ReadDir(ctx.cache)
			for _, e := range entries {
				gotest.NotContains(it, e.Name(), ".tmp-")
			}
		})
	})
	t.When("a cached binary no longer matches its record", func(w *gotest.T) {
		entry := filepath.Join(ctx.cache, gotestrunner.ExportSanitizePkgName("m/a")+".test")
		gotest.NoError(w, os.WriteFile(entry, []byte("v1\x00"), 0o600))
		staged, err := gotestrunner.ExportStageGroup([]string{"m/a"}, filepath.Join(ctx.stage, "d"), ctx.cache)
		w.It("leaves its slot empty, for go to link it again", func(it *gotest.T) {
			gotest.NoError(it, err)
			gotest.False(it, gotestrunner.ExportPlaced(&staged[0]))
		})
	})
	t.When("a group is staged without a cache", func(w *gotest.T) {
		staged, err := gotestrunner.ExportStageGroup([]string{"m/a"}, filepath.Join(ctx.stage, "n"), "")
		w.It("places nothing and publishes nothing", func(it *gotest.T) {
			gotest.NoError(it, err)
			gotest.Empty(it, staged[0].Cached)
			gotestrunner.ExportPublish(&staged[0], gotestrunner.ExportRebuilt)
		})
	})
}
