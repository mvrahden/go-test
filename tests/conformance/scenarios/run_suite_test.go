package scenarios_test

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// ScenariosRunTestSuite: --verify judges the run it made by its exit code
// too, not by the scenarios alone.
type ScenariosRunTestSuite struct {
	dir                                    string
	failed, failedJSON, passed, passedJSON testkit.Run
}

func (s *ScenariosRunTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second, SetupTimeout: 90 * time.Second}
}

func (s *ScenariosRunTestSuite) BeforeAll(t *gotest.T) {
	s.dir = tempDir(t, "gotest-scenarios-run-")
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	mod := filepath.Join(s.dir, "mod")
	gotest.NoError(t, testkit.StageModule(root, filepath.Join("..", "testdata", "scenariosrun"), mod))
	fail := []string{"SCENRUNMOD_SETUP=fail"}
	s.failed, err = testkit.RunCLIWithEnv(t.Context(), mod, fail, "scenarios", "--verify")
	gotest.NoError(t, err)
	s.failedJSON, err = testkit.RunCLIWithEnv(t.Context(), mod, fail, "scenarios", "--verify", "--format=json")
	gotest.NoError(t, err)
	s.passed = scenarios(t, mod, "--verify")
	s.passedJSON = scenarios(t, mod, "--verify", "--format=json")
}

func (s *ScenariosRunTestSuite) AfterAll(t *gotest.T) {
	if s.dir != "" {
		gotest.NoError(t, os.RemoveAll(s.dir))
	}
}

func (s *ScenariosRunTestSuite) TestARunOfItsOwn(t *gotest.T) {
	t.When("--verify runs bound packages whose BeforeAll fails", func(t *gotest.T) {
		t.It("exits 1 under the default policy, the closing line naming FAIL and evidence: run, exited 1", func(it *gotest.T) {
			gotest.Equal(it, 1, s.failed.Exit, "stderr: %s", s.failed.Stderr)
			gotest.Regexp(it, "(?m)^gotest scenarios: FAIL exit 1 \\| scenarios: 1 \\(1 not-run\\) \\| evidence: run, exited 1 \\| fail-on: ambiguous,failing$", s.failed.Stdout)
		})
		t.It("lists the run on a failing line of its own and marks it failed in the JSON document", func(it *gotest.T) {
			gotest.Equal(it, "failing    the run of the bound packages exited 1", strings.Split(s.failed.Stdout, "\n")[0])
			doc := readDoc(it, &s.failedJSON)
			gotest.NotZero(it, doc.Run)
			gotest.True(it, doc.Run.Failed)
			gotest.NotZero(it, doc.Run.ExitCode)
			gotest.Equal(it, 1, *doc.Run.ExitCode)
			gotest.Equal(it, map[string]int{"not-run": 1}, doc.Summary.Statuses)
			gotest.Equal(it, 1, doc.Schema)
		})
	})
	t.When("--verify runs bound packages that pass", func(t *gotest.T) {
		t.It("exits 0, the closing line naming evidence: run, and marks no run failed", func(it *gotest.T) {
			gotest.Equal(it, 0, s.passed.Exit, "stderr: %s", s.passed.Stderr)
			gotest.Equal(it, "gotest scenarios: PASS exit 0 | scenarios: 1 (1 verified) | evidence: run | fail-on: ambiguous,failing\n", s.passed.Stdout)
			doc := readDoc(it, &s.passedJSON)
			gotest.NotZero(it, doc.Run)
			gotest.False(it, doc.Run.Failed)
			gotest.NotContains(it, s.passedJSON.Stdout, "\"failed\"")
		})
	})
}
