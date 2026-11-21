package cli_test

import (
	"debug/elf"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"time"

	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLILinkFlagsTestSuite: how a run links its test binaries.
type CLILinkFlagsTestSuite struct{}

func (s *CLILinkFlagsTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Timeout: 90 * time.Second}
}

// hasDebugInfo reports whether the one binary a run left in cache carries
// DWARF.
func hasDebugInfo(it *gotest.T, cache string) bool {
	bins, _ := filepath.Glob(filepath.Join(cache, "bin", "*", "*.test"))
	gotest.Len(it, bins, 1)
	f, err := elf.Open(bins[0])
	gotest.NoError(it, err)
	found := f.Section(".debug_info") != nil || f.Section(".zdebug_info") != nil
	gotest.NoError(it, f.Close())
	return found
}

func (s *CLILinkFlagsTestSuite) TestHowTestBinariesAreLinked(t *gotest.T) {
	if runtime.GOOS != "linux" {
		t.Skipf("reads ELF sections; the flag table runs on every platform")
	}
	root := gotest.Must(testkit.RepoRoot())
	dir := gotest.Must(testkit.StagePackage(root, filepath.Join("..", "testdata", "staleprobe"), t.TempDir(), "staleprobe.example"))
	run := func(env []string, args ...string) (testkit.Run, string) {
		cache := t.TempDir()
		r, err := testkit.RunCLIWithEnv(t.Context(), dir, append([]string{protocol.EnvCacheDir + "=" + cache}, env...), append([]string{"."}, args...)...)
		gotest.NoError(t, err)
		gotest.Equal(t, 0, r.Exit, "stderr: %s", r.Stderr)
		return r, cache
	}
	_, plain := run(nil)
	profile := filepath.Join(t.TempDir(), "cpu.out")
	_, profiled := run(nil, "-cpuprofile", profile)
	fromEnv, goflags := run([]string{"GOFLAGS=-ldflags=-X=staleprobe.example/staleprobe.linked=fromenv"}, "-v")

	t.When("a package is run with no link or profile flag", func(w *gotest.T) {
		w.It("its test binary carries no debug information", func(it *gotest.T) {
			gotest.False(it, hasDebugInfo(it, plain))
		})
	})
	t.When("the package is run with -cpuprofile", func(w *gotest.T) {
		w.It("its test binary carries debug information", func(it *gotest.T) {
			gotest.True(it, hasDebugInfo(it, profiled))
			info, err := os.Stat(profile)
			gotest.NoError(it, err)
			gotest.Greater(it, info.Size(), int64(0))
		})
	})
	t.When("GOFLAGS sets -ldflags=-X for a variable the test prints", func(w *gotest.T) {
		w.It("the test prints the value and the binary carries debug information", func(it *gotest.T) {
			gotest.Regexp(it, regexp.QuoteMeta("stale-probe: dep=1 test=1 tag=off linked=fromenv"), fromEnv.Stdout)
			gotest.True(it, hasDebugInfo(it, goflags))
		})
	})
}
