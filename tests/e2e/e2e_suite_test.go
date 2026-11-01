package e2e_test

import (
	"path/filepath"
	"regexp"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/e2e/internal/testutils"
)

// E2ETestSuite is the smoke of a consumer-shaped module: a copy of this
// repository whose examples module reaches gotest through a go.work, run the
// way a user runs it. Every other claim about the CLI lives in specs/, bound
// to the conformance suites.
// Sequential: its one method runs against a module copy built in BeforeAll.
type E2ETestSuite struct {
	workDir string
}

func (s *E2ETestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Timeout: 3 * time.Minute, SetupTimeout: 3 * time.Minute}
}

func (s *E2ETestSuite) BeforeAll(t *gotest.T) {
	s.workDir = t.TempDir()
	testutils.CopyModule(t, s.workDir, filepath.Join("..", ".."))
	testutils.UseExamplesWorkspace(t, s.workDir)
}

func (s *E2ETestSuite) AfterAll(t *gotest.T) {}

// examples are the example packages that declare suites.
var examples = []string{
	"auth", "benchmarking", "cart", "datarace", "fuzzing", "notification",
	"repository", "repository/inventory", "repository/orders", "search",
}

func (s *E2ETestSuite) TestTheExamplesModule(t *gotest.T) {
	run, err := testkit.RunCLI(t.Context(), filepath.Join(s.workDir, "examples"), "github.com/mvrahden/go-test/examples/...")
	gotest.NoError(t, err)
	t.When("its packages are run by an import path pattern", func(w *gotest.T) {
		w.It("passes and reports each example package ok", func(it *gotest.T) {
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			for _, pkg := range examples {
				gotest.Regexp(it, `(?m)^ok\s+github\.com/mvrahden/go-test/examples/`+regexp.QuoteMeta(pkg)+`\s`, run.Stdout)
			}
		})
	})
}
