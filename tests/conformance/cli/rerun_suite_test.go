package cli_test

import (
	"time"

	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLIRerunTestSuite: gotest keeps test binaries between runs, never results.
type CLIRerunTestSuite struct{}

func (s *CLIRerunTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 60 * time.Second}
}

// testsStarted counts the tests a run started.
func testsStarted(r *testkit.Run) int {
	n := 0
	for _, e := range r.Events {
		if e.Action == "run" {
			n++
		}
	}
	return n
}

func (s *CLIRerunTestSuite) TestEveryRunExecutesItsTests(t *gotest.T) {
	env := []string{protocol.EnvCacheDir + "=" + t.TempDir()}
	markers := t.TempDir()
	first, err := testkit.RunPackageWithEnv(t.Context(), markers, env, "../testdata/passing/")
	gotest.NoError(t, err)
	second, err := testkit.RunPackageWithEnv(t.Context(), markers, env, "../testdata/passing/")
	gotest.NoError(t, err)
	t.When("a package is run twice with nothing changed in between", func(w *gotest.T) {
		w.It("the second run executes its tests again and marks nothing cached", func(it *gotest.T) {
			gotest.Equal(it, 0, first.Exit, first.Stderr)
			gotest.Equal(it, 0, second.Exit, second.Stderr)
			gotest.Greater(it, testsStarted(&first), 0)
			gotest.Equal(it, testsStarted(&first), testsStarted(&second))
			gotest.NotContains(it, second.Stdout, "(cached)")
		})
	})
}
