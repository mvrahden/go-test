package main_test

import (
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

func (s *BinaryCacheTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.IntegrationSuiteConfig()
}

func (s *BinaryCacheTestSuite) BeforeAll(t *gotest.T) {
	s.cli = newCLIRunner(s.CLI)
}

const cachedPkg = "./tests/canary/testdata/passing/"

func cacheEnv(dir string) []string {
	return append(os.Environ(), "GOTEST_CACHE_DIR="+dir)
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
