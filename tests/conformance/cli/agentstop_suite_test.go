package cli_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLIAgentStopTestSuite: gotest hook stop tells an agent, once, what its
// change weakened.
type CLIAgentStopTestSuite struct {
	dir   string
	cache []string
	// first and again are two ends of a turn in the checkout whose origin
	// names its default branch; noDefault's runs, in one whose origin does
	// not, the first with stop_hook_active set.
	first, again, active, noDefault testkit.Run
	unjudged                        map[string]testkit.Run
}

func (s *CLIAgentStopTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second, SetupTimeout: 2 * time.Minute}
}

// stopFile is the fixture's test file, relative to its module.
const stopFile = "stop/stop_test.go"

// stopGit runs git in dir, outside any checkout above it.
func stopGit(t *gotest.T, dir string, args ...string) string {
	cmd := exec.CommandContext(t.Context(), "git", args...) //nolint:gosec // G204: git on the suite's own checkouts
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CEILING_DIRECTORIES="+filepath.Dir(dir), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	gotest.NoError(t, err, "git %v: %s", args, out)
	return strings.TrimSpace(string(out))
}

// editStop replaces old with new in the fixture's test file in dir.
func editStop(t *gotest.T, dir, old, new string) {
	file := filepath.Join(dir, filepath.FromSlash(stopFile))
	data, err := os.ReadFile(file) //nolint:gosec // G304: the suite's own checkout
	gotest.NoError(t, err)
	gotest.Contains(t, string(data), old)
	gotest.NoError(t, os.WriteFile(file, []byte(strings.Replace(string(data), old, new, 1)), 0o600))
}

// weakenedCheckout stages the fixture as a checkout whose base commit,
// with origin's default branch at it when withDefault, is followed by a
// commit that removes TestSubtracts and by uncommitted changes that
// exclude TestAdds and change TestMultiplies' body.
func (s *CLIAgentStopTestSuite) weakenedCheckout(t *gotest.T, name string, withDefault bool) string {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	dir := filepath.Join(s.dir, name)
	gotest.NoError(t, testkit.StageModule(root, filepath.Join("..", "testdata", "cliagentstop"), dir))
	stopGit(t, dir, "init", "-q")
	stopGit(t, dir, "add", "-A")
	commit := []string{"-c", "user.name=gotest", "-c", "user.email=gotest@example.com", "commit", "-q", "-am"}
	stopGit(t, dir, append(commit, "base")...)
	if withDefault {
		stopGit(t, dir, "update-ref", "refs/remotes/origin/main", "HEAD")
		stopGit(t, dir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	}
	editStop(t, dir, "func (s *StopTestSuite) TestSubtracts(t *gotest.T) {\n\tgotest.Equal(t, 0, 1-1)\n}\n\n", "")
	stopGit(t, dir, append(commit, "drop")...)
	editStop(t, dir, "TestAdds", "X_TestAdds")
	editStop(t, dir, "gotest.Equal(t, 4, 2*2)", "gotest.Equal(t, 6, 2*3)")
	return dir
}

// stop runs gotest hook stop in dir with the suite's cache.
func (s *CLIAgentStopTestSuite) stop(t *gotest.T, dir string, active bool) testkit.Run {
	data, err := json.Marshal(map[string]any{"hook_event_name": "Stop", "cwd": dir, "stop_hook_active": active})
	gotest.NoError(t, err)
	run, err := testkit.RunCLIWithInputEnv(t.Context(), dir, string(data), s.cache, "hook", "stop")
	gotest.NoError(t, err)
	return run
}

func (s *CLIAgentStopTestSuite) BeforeAll(t *gotest.T) {
	dir, err := os.MkdirTemp("", "gotest-agent-stop-")
	gotest.NoError(t, err)
	s.dir = dir
	s.cache = []string{"GOTEST_CACHE_DIR=" + filepath.Join(dir, "cache"), "GIT_CEILING_DIRECTORIES=" + dir}

	withDefault := s.weakenedCheckout(t, "default", true)
	s.first = s.stop(t, withDefault, false)
	s.again = s.stop(t, withDefault, false)

	noDefault := s.weakenedCheckout(t, "nodefault", false)
	s.active = s.stop(t, noDefault, true)
	s.noDefault = s.stop(t, noDefault, false)

	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	noGit := filepath.Join(dir, "nogit")
	gotest.NoError(t, testkit.StageModule(root, filepath.Join("..", "testdata", "cliagentstop"), noGit))
	editStop(t, noGit, "TestAdds", "X_TestAdds")
	plain := filepath.Join(dir, "plain")
	gotest.NoError(t, testkit.StageModule(root, filepath.Join("..", "testdata", "cliagenthookplain"), plain))
	stopGit(t, plain, "init", "-q")
	s.unjudged = map[string]testkit.Run{
		"outside a git checkout":     s.stop(t, noGit, false),
		"in a module without gotest": s.stop(t, plain, false),
	}
}

func (s *CLIAgentStopTestSuite) AfterAll(t *gotest.T) {
	if s.dir != "" {
		gotest.NoError(t, os.RemoveAll(s.dir))
	}
}

// stopDecision is a stop hook's answer, read back from stdout.
type stopDecision struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

func readDecision(t *gotest.T, run *testkit.Run) stopDecision {
	gotest.Equal(t, 0, run.Exit, "stderr: %s", run.Stderr)
	gotest.Empty(t, run.Stderr)
	var d stopDecision
	gotest.NoError(t, json.Unmarshal([]byte(run.Stdout), &d), "stdout: %s", run.Stdout)
	gotest.Equal(t, "block", d.Decision)
	return d
}

func (s *CLIAgentStopTestSuite) TestTheEndOfATurn(t *gotest.T) {
	t.When("an agent ends its turn in a checkout whose commits and uncommitted changes since the merge base with origin's default branch remove and exclude tests and change a body", func(w *gotest.T) {
		w.It("exits 0 with a block decision whose reason lists the removed and the excluded test with class, subject, values and location, and asks to undo each or state it to the user", func(it *gotest.T) {
			d := readDecision(it, &s.first)
			gotest.Contains(it, d.Reason, "the merge base with origin/main")
			gotest.Contains(it, d.Reason, "\n- removed: ./stop Stop / Subtracts (test, stop/stop_test.go:12)\n")
			gotest.Contains(it, d.Reason, "\n- excluded: ./stop Stop / Adds: TestAdds → X_TestAdds (stop/stop_test.go:8)\n")
			gotest.Contains(it, d.Reason, "either undo it, or state it plainly to the user in your final message")
		})
		w.It("lists no changed body", func(it *gotest.T) {
			d := readDecision(it, &s.first)
			gotest.NotContains(it, d.Reason, "Multiplies")
			gotest.Equal(it, 2, strings.Count(d.Reason, "\n- "))
		})
	})
	t.When("the agent ends its turn again with the same weakenings", func(w *gotest.T) {
		w.It("exits 0 and writes nothing", func(it *gotest.T) {
			gotest.Equal(it, 0, s.again.Exit, "stderr: %s", s.again.Stderr)
			gotest.Empty(it, s.again.Stdout)
			gotest.Empty(it, s.again.Stderr)
		})
	})
	t.When("an agent ends its turn in a checkout whose origin names no default branch", func(w *gotest.T) {
		w.It("lists only the weakenings against HEAD", func(it *gotest.T) {
			d := readDecision(it, &s.noDefault)
			gotest.Contains(it, d.Reason, "(HEAD)")
			gotest.Contains(it, d.Reason, "\n- excluded: ./stop Stop / Adds: TestAdds → X_TestAdds (stop/stop_test.go:8)\n")
			gotest.NotContains(it, d.Reason, "Subtracts")
		})
	})
	t.When("an agent ends its turn with stop_hook_active set, outside a git checkout, or in a module without gotest", func(w *gotest.T) {
		w.It("exits 0 and writes nothing", func(it *gotest.T) {
			runs := map[string]testkit.Run{"with stop_hook_active set": s.active}
			for name, run := range s.unjudged {
				runs[name] = run
			}
			gotest.Len(it, runs, 3)
			for name, run := range runs {
				gotest.Equal(it, 0, run.Exit, "%s: stderr: %s", name, run.Stderr)
				gotest.Empty(it, run.Stdout, name)
				gotest.Empty(it, run.Stderr, name)
			}
		})
	})
}
