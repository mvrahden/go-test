package cli_test

import (
	"time"

	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLIBuildSetTestSuite: a run builds only the packages whose suites it
// dispatches.
type CLIBuildSetTestSuite struct{}

func (s *CLIBuildSetTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Timeout: 60 * time.Second}
}

func (s *CLIBuildSetTestSuite) TestWhatAFilteredRunBuilds(t *gotest.T) {
	env := []string{protocol.EnvCacheDir + "=" + t.TempDir()}
	filtered, err := testkit.RunCLIWithEnv(t.Context(), ".", env, "../testdata/buildset/...", "-run", "^TestSelTestSuite$")
	gotest.NoError(t, err)
	full, err := testkit.RunCLIWithEnv(t.Context(), ".", env, "../testdata/buildset/...")
	gotest.NoError(t, err)
	t.When("-run names a suite of one package and another package's suite fails vet", func(w *gotest.T) {
		w.It("exits 0 and reports the other package as having no tests to run", func(it *gotest.T) {
			gotest.Equal(it, 0, filtered.Exit, "stderr: %s", filtered.Stderr)
			gotest.Regexp(it, `(?m)^ok\s+\S+/buildset/vetfail\s+0\.000s \[no tests to run\]$`, filtered.Stdout)
			gotest.NotContains(it, filtered.Stdout+filtered.Stderr, "fmt.Printf format")
		})
	})
	t.When("the two packages are run without -run", func(w *gotest.T) {
		w.It("exits 2 and reports the vet finding under its package", func(it *gotest.T) {
			gotest.Equal(it, 2, full.Exit)
			gotest.Contains(it, full.Stdout+full.Stderr, `fmt.Printf format %d has arg "not a number" of wrong type string`)
		})
	})
}
