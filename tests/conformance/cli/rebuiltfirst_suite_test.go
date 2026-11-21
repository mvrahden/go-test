package cli_test

import (
	"os"
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLIRebuiltFirstTestSuite: of the packages ready at once, those go rebuilt
// start before those whose cached binary it confirmed.
type CLIRebuiltFirstTestSuite struct{}

func (s *CLIRebuiltFirstTestSuite) SuiteConfig() gotest.SuiteConfig {
	// Two runs of four one-test packages, one process at a time.
	return gotest.SuiteConfig{Timeout: 60 * time.Second}
}

// ranIn runs the staged packages one process at a time with the binary cache
// in cache and returns the order their suites ran in.
func ranIn(t *gotest.T, dir, cache string) []string {
	markers := t.TempDir()
	env := []string{protocol.EnvCacheDir + "=" + cache, "GOTEST_CANARY_DIR=" + markers}
	r, err := testkit.RunCLIWithEnv(t.Context(), dir, env, "-p", "1", "./...")
	gotest.NoError(t, err)
	gotest.Equal(t, 0, r.Exit, "stderr: %s", r.Stderr)
	order, err := testkit.ReadMarkers(filepath.Join(markers, testkit.MarkersFile))
	gotest.NoError(t, err)
	return order
}

func (s *CLIRebuiltFirstTestSuite) TestPackagesGoRebuilt(t *gotest.T) {
	root := gotest.Must(testkit.RepoRoot())
	dir := gotest.Must(testkit.StagePackage(root, filepath.Join("..", "testdata", "rebuiltfirst"), t.TempDir(), "rebuiltfirst.example"))
	cache := t.TempDir()
	first := ranIn(t, dir, cache)
	// A new method: an edit go would compile to the same binary is no edit.
	path := filepath.Join(dir, "d", "suite_test.go")
	src := string(gotest.Must(os.ReadFile(path)))
	gotest.NoError(t, os.WriteFile(path, []byte(src+"\nfunc (s *DTestSuite) TestEdited(t *gotest.T) {}\n"), 0o600))
	again := ranIn(t, dir, cache)
	t.When("four packages ran once, the last of them is edited, and they run again with -p 1", func(w *gotest.T) {
		w.It("the edited package's suite runs first", func(it *gotest.T) {
			gotest.ElementsMatch(it, []string{"a", "b", "c", "d"}, first)
			gotest.Equal(it, []string{"d", "a", "b", "c"}, again)
		})
	})
}
