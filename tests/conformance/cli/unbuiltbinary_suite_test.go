package cli_test

import (
	"os"
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLIUnbuiltBinaryTestSuite: a run reports only from a binary the go command
// built or confirmed for it. Each method stages the probe package and a
// cache of its own.
type CLIUnbuiltBinaryTestSuite struct{}

func (s *CLIUnbuiltBinaryTestSuite) SuiteConfig() gotest.SuiteConfig {
	// One build for the host, then the standard library once for another GOOS.
	return gotest.SuiteConfig{Parallel: true, Timeout: 2 * time.Minute}
}

func (s *CLIUnbuiltBinaryTestSuite) BeforeEach(t *gotest.T) *staleCtx {
	root := gotest.Must(testkit.RepoRoot())
	dir := gotest.Must(testkit.StagePackage(root, filepath.Join("..", "testdata", "staleprobe"), t.TempDir(), "staleprobe.example"))
	return &staleCtx{dir: dir, cache: t.TempDir()}
}

const dryRunRefusal = "FAIL: -n in GOFLAGS is not supported: gotest has no dry run; drop it from GOFLAGS (the environment, or go env -w), and use --debug to keep the generated sources and the work dir of a real run"

func (s *CLIUnbuiltBinaryTestSuite) TestADryRunThroughGOFLAGS(t *gotest.T, c *staleCtx) {
	gotest.Equal(t, "stale-probe: dep=1 test=1 tag=off linked=", c.run(t, nil))
	c.edit(t, "suite_test.go", "testValue = 1", "testValue = 2")
	viaEnv := c.cli(t, []string{"GOFLAGS=-n"})
	envFile := filepath.Join(t.TempDir(), "go.env")
	gotest.NoError(t, os.WriteFile(envFile, []byte("GOFLAGS=-n\n"), 0o600))
	viaFile := c.cli(t, []string{"GOFLAGS=", "GOENV=" + envFile})
	t.When("GOFLAGS carries -n for a run after a test file changed since a passing run", func(w *gotest.T) {
		w.It("refuses the run with exit 2 before anything is built, naming GOFLAGS, and prints nothing on stdout", func(it *gotest.T) {
			gotest.Equal(it, 2, viaEnv.Exit, "stdout: %s\nstderr: %s", viaEnv.Stdout, viaEnv.Stderr)
			gotest.Contains(it, viaEnv.Stderr, dryRunRefusal)
			gotest.Empty(it, viaEnv.Stdout)
		})
		w.It("refuses it the same when go's env file sets GOFLAGS", func(it *gotest.T) {
			gotest.Equal(it, 2, viaFile.Exit, "stdout: %s\nstderr: %s", viaFile.Stdout, viaFile.Stderr)
			gotest.Contains(it, viaFile.Stderr, dryRunRefusal)
			gotest.Empty(it, viaFile.Stdout)
		})
	})
}
