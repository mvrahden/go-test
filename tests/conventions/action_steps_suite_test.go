package conventions_test

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// ActionStepsTestSuite runs the action's steps under a stand-in CLI and go,
// so every input no CI job sets is read back from the command it leads to.
type ActionStepsTestSuite struct{}

func (s *ActionStepsTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second}
}

func (s *ActionStepsTestSuite) SuiteGuard() string {
	if reason := unixActionGuard(); reason != "" {
		return reason
	}
	for _, tool := range []string{"git", "python3"} {
		if _, err := exec.LookPath(tool); err != nil {
			return tool + " not found"
		}
	}
	return ""
}

// loggingCLI logs its arguments, prints STANDIN_STDOUT and exits STANDIN_EXIT.
const loggingCLI = `#!/usr/bin/env bash
echo "$*" >> "$STANDIN_LOG"
[ -n "$STANDIN_STDOUT" ] && printf '%s\n' "$STANDIN_STDOUT"
exit "${STANDIN_EXIT:-0}"
`

// coverGo answers `go tool cover -func=<profile>` with a total and logs every other call.
const coverGo = `#!/usr/bin/env bash
if [ "$1 $2" = "tool cover" ]; then printf 'total:\t(statements)\t81.5%%\n'; exit 0; fi
echo "$*" >> "$GO_LOG"
`

// step runs the action step key in a fresh directory; see stepIn.
func (s *ActionStepsTestSuite) step(t *gotest.T, key string, env ...string) (actionRun, string) {
	return s.stepIn(t, t.TempDir(), key, env...)
}

// stepIn runs the action step key in dir with loggingCLI as GOTEST_BIN and
// coverGo as go, and returns the run and the CLI's one call, empty when it
// was not started.
func (s *ActionStepsTestSuite) stepIn(t *gotest.T, dir, key string, env ...string) (actionRun, string) {
	tools := t.TempDir()
	cli, log := filepath.Join(tools, "gotest"), filepath.Join(tools, "calls")
	gotest.NoError(t, os.WriteFile(cli, []byte(loggingCLI), 0o700)) //nolint:gosec // G306: an executable
	run := runActionStep(t, actionStep(t, key), dir, map[string]string{"go": coverGo},
		append([]string{"INPUT_VERSION=gomod", "GOTEST_BIN=" + cli, "STANDIN_LOG=" + log,
			"GO_LOG=" + filepath.Join(tools, "go.log")}, env...)...)
	calls, _ := os.ReadFile(log)
	return run, strings.TrimSpace(string(calls))
}

func (s *ActionStepsTestSuite) test(t *gotest.T, env ...string) (actionRun, string) {
	return s.step(t, "test", append([]string{"INPUT_PACKAGES=./..."}, env...)...)
}

func (s *ActionStepsTestSuite) TestCoverageAddsOneProfileAndReportsItsTotal(t *gotest.T) {
	run, call := s.test(t, "INPUT_COVERAGE=true")
	profile := regexp.MustCompile(`--coverage=(\S+)`).FindStringSubmatch(call)
	gotest.Len(t, profile, 2, "call: %s", call)
	gotest.Contains(t, call, " -coverprofile="+profile[1])
	gotest.Contains(t, run.output, "coverage=81.5\n")
	_, err := os.Stat(profile[1])
	gotest.ErrorIs(t, err, fs.ErrNotExist, "the temporary profile is removed")
}

func (s *ActionStepsTestSuite) TestGoTestFlagsFollowThePackagesAndTheirProfileWins(t *gotest.T) {
	_, call := s.test(t, "INPUT_COVERAGE=true", "INPUT_RACE=true", "INPUT_GO_TEST_FLAGS=-short\n-coverprofile=mine.out")
	gotest.Regexp(t, `^summary --github --coverage=mine\.out \./\.\.\. -race -short -coverprofile=mine\.out$`, call)
}

func (s *ActionStepsTestSuite) TestGoTestFlagsWithQuotesAreRefused(t *gotest.T) {
	run, call := s.test(t, `INPUT_GO_TEST_FLAGS=-run "A B"`)
	gotest.Equal(t, 2, run.exit)
	gotest.Contains(t, run.stdout, "::error::go-test-flags must not contain quotes: one entry per word or line")
	gotest.Empty(t, call)
}

func (s *ActionStepsTestSuite) TestAVersionIsInstalled(t *gotest.T) {
	log := filepath.Join(t.TempDir(), "go.log")
	run := runActionStep(t, actionStep(t, "Install gotest"), t.TempDir(), map[string]string{"go": coverGo},
		"GOTEST_VERSION=v1.36.0", "GO_LOG="+log)
	gotest.Equal(t, 0, run.exit, "stderr: %s", run.stderr)
	data, err := os.ReadFile(log)
	gotest.NoError(t, err)
	gotest.Equal(t, "install github.com/mvrahden/go-test/cmd/gotest@v1.36.0\n", string(data))
}

func (s *ActionStepsTestSuite) TestBenchInputsReachTheBenchCommand(t *gotest.T) {
	run, call := s.step(t, "bench", "INPUT_PACKAGES=./...", "INPUT_FLAGS=-count=4",
		"INPUT_BENCH_BASELINE=base.json", "INPUT_BENCH_GATE=10", "INPUT_BENCH_SAVE=out.json",
		`STANDIN_STDOUT={"gate":{"breachedKeys":["pkg.BenchmarkA","pkg.BenchmarkB"]}}`, "STANDIN_EXIT=1")
	gotest.Equal(t, "bench --spec --json --against=base.json --gate=10 --save=out.json -count=4 ./...", call)
	gotest.Equal(t, 1, run.exit)
	gotest.Contains(t, run.output, "bench-breached-keys=pkg.BenchmarkA,pkg.BenchmarkB\n")
	gotest.Regexp(t, `(?m)^bench-report=\S+$`, run.output)
}

func (s *ActionStepsTestSuite) TestBenchSaveDistinguishesOffFromTheConfiguredBaseline(t *gotest.T) {
	_, off := s.step(t, "bench", "INPUT_PACKAGES=./...", "INPUT_BENCH_SAVE=false")
	gotest.Equal(t, "bench --spec --json ./...", off)
	_, configured := s.step(t, "bench", "INPUT_PACKAGES=./...", "INPUT_BENCH_SAVE=")
	gotest.Equal(t, "bench --spec --json --save= ./...", configured)
}

func (s *ActionStepsTestSuite) TestFuzzReportsItsCrashersRelativeToTheCheckout(t *gotest.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	gotest.NoError(t, err)
	run, call := s.stepIn(t, dir, "fuzz", "INPUT_PACKAGES=./...", "INPUT_FUZZ_FOR=30s",
		"STANDIN_STDOUT=[FuzzParse] new crasher: "+filepath.Join(dir, "testdata", "fuzz", "FuzzParse", "abc"), "STANDIN_EXIT=1")
	gotest.Equal(t, "fuzz --for=30s ./...", call)
	gotest.Equal(t, 1, run.exit)
	gotest.Contains(t, run.output, "fuzz-crashers=testdata/fuzz/FuzzParse/abc\n")
	_, bare := s.step(t, "fuzz", "INPUT_PACKAGES=./...", "INPUT_FUZZ_FOR=")
	gotest.Equal(t, "fuzz ./...", bare)
}

func (s *ActionStepsTestSuite) TestTheFuzzCacheStepsFollowTheirInput(t *gotest.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "action.yml"))
	gotest.NoError(t, err)
	var action struct {
		Runs struct {
			Steps []workflowStep `yaml:"steps"`
		} `yaml:"runs"`
	}
	gotest.NoError(t, yaml.Unmarshal(data, &action))
	ifs := map[string]string{}
	for _, st := range action.Runs.Steps {
		ifs[st.Name] = st.If
	}
	for _, name := range []string{"Locate the Go fuzz cache", "Restore the fuzz cache", "Save the fuzz cache"} {
		gotest.Contains(t, ifs[name], "inputs.fuzz-cache == 'true'", "step %q", name)
	}
}

// git runs git in dir with no user or system configuration and returns its
// trimmed output.
func (s *ActionStepsTestSuite) git(t *gotest.T, dir string, args ...string) string {
	cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	gotest.NoError(t, err, "git %v: %s", args, out)
	return strings.TrimSpace(string(out))
}

func (s *ActionStepsTestSuite) TestTheBadgeGoesToTheNamedBranchOfTheDefaultBranchOnly(t *gotest.T) {
	bare := filepath.Join(t.TempDir(), "remote.git")
	gotest.NoError(t, os.MkdirAll(bare, 0o750))
	s.git(t, bare, "init", "-q", "--bare")
	badge := filepath.Join(t.TempDir(), "coverage.svg")
	gotest.NoError(t, os.WriteFile(badge, []byte("<svg>81.5</svg>"), 0o600))
	env := []string{"BADGE_FILE=" + badge, "BADGE_BRANCH=metrics", "COVERAGE=81.5", "DEFAULT_BRANCH=main",
		"GH_TOKEN=tok", "GITHUB_REPOSITORY=o/r", "GIT_CONFIG_GLOBAL=" + os.DevNull, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=url.file://" + bare + ".insteadOf", "GIT_CONFIG_VALUE_0=https://x-access-token:tok@github.com/o/r.git"}
	script := actionStep(t, "Publish coverage badge")

	pr := runActionStep(t, script, t.TempDir(), nil, append(env, "GITHUB_REF=refs/pull/7/merge")...)
	gotest.Contains(t, pr.stdout, "Coverage badge is published from main only; skipping on refs/pull/7/merge")
	gotest.Empty(t, s.git(t, bare, "branch", "--list", "metrics"))

	first := runActionStep(t, script, t.TempDir(), nil, append(env, "GITHUB_REF=refs/heads/main")...)
	gotest.Equal(t, 0, first.exit, "stdout: %s\nstderr: %s", first.stdout, first.stderr)
	gotest.Contains(t, first.stdout, "Coverage badge published: https://raw.githubusercontent.com/o/r/metrics/coverage.svg")
	gotest.Equal(t, "<svg>81.5</svg>", s.git(t, bare, "show", "metrics:coverage.svg"))

	again := runActionStep(t, script, t.TempDir(), nil, append(env, "GITHUB_REF=refs/heads/main")...)
	gotest.Contains(t, again.stdout, "Coverage badge unchanged (81.5%)")
}
