package main_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/gotestcli"
)

// BinaryCacheTestSuite: a run links its test binaries into a cache keyed by
// what decides the link, so a second run over unchanged code skips it.
// Sequential: every method drives whole runs through the shared binary.
//
//nolint:lifecycle-pair // BeforeAll only wraps the shared binary, which its fixture removes
type BinaryCacheTestSuite struct {
	CLI *gotestcli.BinarySharedFixture
	cli cliRunner
}

func (s *BinaryCacheTestSuite) BeforeAll(t *gotest.T) {
	s.cli = newCLIRunner(s.CLI)
}

const cachedPkg = "./tests/canary/testdata/passing/"

func cacheEnv(dir string) []string {
	return []string{"GOTEST_CACHE_DIR=" + dir}
}

// linkStep matches a link action in the build trace GOFLAGS=-x makes go
// print; the CLI forwards a successful build's stderr. Windows quotes the
// tool path: `"C:\\...\\link.exe" -o`.
var linkStep = regexp.MustCompile(`(?m)(^|[/\\])link(\.exe)?"? `)

func (s *BinaryCacheTestSuite) TestSecondRunSkipsTheLink(t *gotest.T) {
	cache := t.TempDir()
	env := append(cacheEnv(cache), "GOFLAGS=-x")
	out, code := s.cli.runEnv(t, env, cachedPkg)
	gotest.Equal(t, 0, code, out)
	gotest.Len(t, linkStep.FindAllString(out, -1), 1, out)
	gotest.Len(t, gotest.Must(filepath.Glob(filepath.Join(cache, "bin", "*", "*.test"))), 1)

	out, code = s.cli.runEnv(t, env, cachedPkg)
	gotest.Equal(t, 0, code, out)
	gotest.Empty(t, linkStep.FindAllString(out, -1), out)

	// The cache is what skipped it: without one the link runs again.
	out, code = s.cli.runEnv(t, env, "--no-cache", cachedPkg)
	gotest.Equal(t, 0, code, out)
	gotest.Len(t, linkStep.FindAllString(out, -1), 1, out)
}

func (s *BinaryCacheTestSuite) TestBuildFlagsKeyTheCache(t *gotest.T) {
	cache := t.TempDir()
	out, code := s.cli.runEnv(t, cacheEnv(cache), cachedPkg)
	gotest.Equal(t, 0, code, out)
	out, code = s.cli.runEnv(t, cacheEnv(cache), "-tags=cachekey", cachedPkg)
	gotest.Equal(t, 0, code, out)
	gotest.Len(t, gotest.Must(filepath.Glob(filepath.Join(cache, "bin", "*"))), 2)
}

func (s *BinaryCacheTestSuite) TestNoCacheLeavesNoBinaryBehind(t *gotest.T) {
	cache := t.TempDir()
	out, code := s.cli.runEnv(t, cacheEnv(cache), "--no-cache", cachedPkg)
	gotest.Equal(t, 0, code, out)
	_, err := os.Stat(filepath.Join(cache, "bin"))
	gotest.ErrorIs(t, err, fs.ErrNotExist)
}

// cached is the one test binary a run left in the cache under dir.
func cached(t *gotest.T, dir string) string {
	bins := gotest.Must(filepath.Glob(filepath.Join(dir, "bin", "*", "*.test")))
	gotest.Len(t, bins, 1)
	return bins[0]
}

func (s *BinaryCacheTestSuite) TestADamagedCachedBinary(t *gotest.T) {
	for t, damage := range gotest.Each(t, []struct {
		Desc string
		do   func(path string, size int64) error
	}{
		{"truncated to half", func(p string, n int64) error { return os.Truncate(p, n/2) }},
		{"its last 4 KiB zero-filled", func(p string, n int64) error {
			f, err := os.OpenFile(p, os.O_WRONLY, 0)
			if err != nil {
				return err
			}
			_, err = f.WriteAt(make([]byte, 4096), n-4096)
			return errors.Join(err, f.Close())
		}},
	}) {
		dir := t.TempDir()
		_, code := s.cli.runEnv(t, cacheEnv(dir), cachedPkg)
		gotest.Equal(t, 0, code)
		bin := cached(t, dir)
		before, err := os.Stat(bin)
		gotest.NoError(t, err)
		gotest.NoError(t, damage.do(bin, before.Size()))
		out, code := s.cli.runEnv(t, cacheEnv(dir), cachedPkg)
		after, statErr := os.Stat(bin)
		t.When("a cached test binary is damaged after a run linked it", func(w *gotest.T) {
			w.It("links it again, says so and passes", func(it *gotest.T) {
				gotest.Equal(it, 0, code, out)
				gotest.Contains(it, out, "did not match its record and was linked again")
				gotest.NoError(it, statErr)
				gotest.Equal(it, before.Size(), after.Size())
			})
		})
	}
}

func (s *BinaryCacheTestSuite) TestACachedBinaryWithoutARecord(t *gotest.T) {
	dir := t.TempDir()
	_, code := s.cli.runEnv(t, cacheEnv(dir), cachedPkg)
	gotest.Equal(t, 0, code)
	bin := cached(t, dir)
	gotest.NoError(t, os.Remove(bin+".sum"))
	out, code := s.cli.runEnv(t, append(cacheEnv(dir), "GOFLAGS=-x"), cachedPkg)
	gotest.Equal(t, 0, code, out)
	gotest.NotContains(t, out, "did not match")
	gotest.Len(t, linkStep.FindAllString(out, -1), 1, out)
	_, err := os.Stat(bin + ".sum")
	gotest.NoError(t, err)
}

func (s *BinaryCacheTestSuite) TestAnUntouchedCachedBinary(t *gotest.T) {
	dir := t.TempDir()
	_, code := s.cli.runEnv(t, cacheEnv(dir), cachedPkg)
	gotest.Equal(t, 0, code)
	out, code := s.cli.runEnv(t, append(cacheEnv(dir), "GOFLAGS=-x"), cachedPkg)
	gotest.Equal(t, 0, code, out)
	gotest.NotContains(t, out, "linked again")
	gotest.Empty(t, linkStep.FindAllString(out, -1), out)
}

func (s *BinaryCacheTestSuite) TestCleanCache(t *gotest.T) {
	dir := t.TempDir()
	_, code := s.cli.runEnv(t, cacheEnv(dir), cachedPkg)
	gotest.Equal(t, 0, code)
	out, code := s.cli.runEnv(t, cacheEnv(dir), "clean", "--cache")
	refused, refusedCode := s.cli.runEnv(t, cacheEnv(dir), "clean", "--cache", "./...")
	t.When("gotest clean --cache runs", func(w *gotest.T) {
		w.It("removes the cached binaries and says how many", func(it *gotest.T) {
			gotest.Equal(it, 0, code, out)
			gotest.Contains(it, out, "removed 1 cached test binaries from "+filepath.Join(dir, "bin"))
			gotest.Empty(it, gotest.Must(filepath.Glob(filepath.Join(dir, "bin", "*", "*.test"))))
		})
		w.It("refuses a package beside the flag", func(it *gotest.T) {
			gotest.Equal(it, 2, refusedCode, refused)
			gotest.Contains(it, refused, "--cache clears the test binary cache and takes no packages; run gotest clean <packages> for generated files")
		})
	})
}
