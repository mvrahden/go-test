package cli_test

import (
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLIBatchTestSuite: a run builds its packages together and still reports
// each build failure under its own package.
type CLIBatchTestSuite struct{}

func (s *CLIBatchTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Timeout: 60 * time.Second}
}

// buildOutputOf joins the build output of the builds whose package, the
// ImportPath cut at " [" and without an external test's _test, ends in
// suffix.
func buildOutputOf(out map[string]string, suffix string) string {
	var b strings.Builder
	for build, text := range out {
		pkg, _, _ := strings.Cut(build, " [")
		if strings.HasSuffix(strings.TrimSuffix(pkg, "_test"), suffix) {
			b.WriteString(text)
		}
	}
	return b.String()
}

func (s *CLIBatchTestSuite) TestBuildingSeveralPackages(t *gotest.T) {
	env := []string{protocol.EnvCacheDir + "=" + t.TempDir()}
	mixed, err := testkit.RunCLIWithEnv(t.Context(), ".", env, "-json", "../testdata/batch/ok/", "../testdata/batch/vetone/", "../testdata/batch/vettwo/")
	gotest.NoError(t, err)
	t.When("two packages each fail vet and a third passes", func(w *gotest.T) {
		w.It("exits 2, reports each finding under its own package and runs the third", func(it *gotest.T) {
			gotest.Equal(it, 2, mixed.Exit, "stderr: %s", mixed.Stderr)
			out := map[string]string{} // by ImportPath: a build event carries no Package
			for _, e := range mixed.Events {
				if e.Action == "build-output" {
					out[e.ImportPath] += e.Output
				}
			}
			gotest.Contains(it, buildOutputOf(out, "/batch/vetone"), `has arg "one"`)
			gotest.NotContains(it, buildOutputOf(out, "/batch/vetone"), `has arg "two"`)
			gotest.Contains(it, buildOutputOf(out, "/batch/vettwo"), `has arg "two"`)
			gotest.Equal(it, "pass", mixed.Verdict("TestOkTestSuite/TestPasses"))
		})
	})
	utils, err := testkit.RunCLIWithEnv(t.Context(), ".", env, "-json", "../testdata/batch/a/util/", "../testdata/batch/b/util/")
	gotest.NoError(t, err)
	t.When("two packages named util are run together", func(w *gotest.T) {
		w.It("both run and pass", func(it *gotest.T) {
			gotest.Equal(it, 0, utils.Exit, "stderr: %s", utils.Stderr)
			gotest.Equal(it, "pass", utils.Verdict("TestUtilATestSuite/TestPasses"))
			gotest.Equal(it, "pass", utils.Verdict("TestUtilBTestSuite/TestPasses"))
		})
	})
}
