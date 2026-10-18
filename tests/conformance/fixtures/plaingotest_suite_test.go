package fixtures_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// PlainGoTestTestSuite: a suite that binds a shared fixture has no state
// under plain go test, which only the runner writes.
type PlainGoTestTestSuite struct {
	exit int
	out  string
}

func (s *PlainGoTestTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.IntegrationSuiteConfig()
}

// BeforeAll stages the protocol package into a module of its own, generates
// its harness on disk and runs plain go test on it.
func (s *PlainGoTestTestSuite) BeforeAll(t *gotest.T) {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	cli, err := testkit.BuildCLI(t.Context(), root, t.TempDir())
	gotest.NoError(t, err)
	testkit.ScrubActionsEnv()
	dir := t.TempDir()
	_, err = testkit.StagePackage(root, filepath.Join("..", "testdata", "protocol"), dir, "example.com/plain")
	gotest.NoError(t, err)
	generate := exec.Command(cli, "generate", "./protocol/") //nolint:gosec // G204: controlled binary with fixed args
	generate.Dir = dir
	out, err := generate.CombinedOutput()
	gotest.NoError(t, err, string(out))

	run := exec.Command("go", "test", "-count=1", "-v", "./protocol/")
	run.Dir = dir
	run.Env = withoutStateFile(os.Environ())
	raw, err := run.CombinedOutput()
	s.out = string(raw)
	if err != nil {
		exit := gotest.ErrorAs[*exec.ExitError](t, err)
		s.exit = exit.ExitCode()
	}
}

// AfterAll has nothing to release: the binary and the module live in temp dirs.
func (s *PlainGoTestTestSuite) AfterAll(t *gotest.T) {}

func (s *PlainGoTestTestSuite) TestSharedFixtureSuite(t *gotest.T) {
	t.When("a suite binding a shared fixture runs under plain go test", func(w *gotest.T) {
		w.It("fails naming the shared fixture it binds and go tool gotest, before any method runs", func(it *gotest.T) {
			gotest.Equal(it, 1, s.exit, s.out)
			gotest.Regexp(it, `(?m)^\s+\S+: fixture setup: binds shared fixture StdoutSharedFixture; run via go tool gotest$`, s.out)
			gotest.NotContains(it, s.out, "=== RUN   TestStdoutTestSuite/TestStateArrived")
		})
	})
}

// withoutStateFile drops the runner's state file variable from env, as a
// plain go test has none.
func withoutStateFile(env []string) []string {
	kept := env[:0:0]
	for _, kv := range env {
		if !strings.HasPrefix(kv, protocol.EnvSharedStateFile+"=") {
			kept = append(kept, kv)
		}
	}
	return kept
}
