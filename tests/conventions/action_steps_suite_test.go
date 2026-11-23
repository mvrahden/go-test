package conventions_test

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go.yaml.in/yaml/v4"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// ActionStepsTestSuite runs the action's steps under a stand-in CLI and go,
// with each step's environment built from its env: mapping in action.yml,
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

// actionFile is the part of action.yml a step's environment is built from.
type actionFile struct {
	Inputs map[string]struct {
		Default string `yaml:"default"`
	} `yaml:"inputs"`
	Runs struct {
		Steps []workflowStep `yaml:"steps"`
	} `yaml:"runs"`
}

func readAction(t *gotest.T) actionFile {
	data, err := os.ReadFile(filepath.Join("..", "..", "action.yml"))
	gotest.NoError(t, err)
	var action actionFile
	gotest.NoError(t, yaml.Unmarshal(data, &action))
	return action
}

// actionStepNamed is the step with that id, or, when no step has it, that name.
func actionStepNamed(t *gotest.T, action actionFile, key string) workflowStep {
	for i := range action.Runs.Steps {
		if action.Runs.Steps[i].ID == key {
			return action.Runs.Steps[i]
		}
	}
	for i := range action.Runs.Steps {
		if action.Runs.Steps[i].Name == key {
			return action.Runs.Steps[i]
		}
	}
	gotest.Fail(t, "action.yml has no step with id or name "+key)
	return workflowStep{}
}

var inputExpr = regexp.MustCompile(`^\$\{\{\s*inputs\.([\w-]+)\s*\}\}$`)

// inputEnv is the environment GitHub gives the step for these inputs: every
// variable its env: maps from an input, set to the given value or the
// input's default. Each given input must be read by the step.
func inputEnv(t *gotest.T, action actionFile, st *workflowStep, inputs map[string]string) []string {
	read := map[string]bool{}
	var env []string
	for name, expr := range st.Env {
		m := inputExpr.FindStringSubmatch(expr)
		if m == nil {
			continue
		}
		input, ok := action.Inputs[m[1]]
		gotest.True(t, ok, "step %q maps %s to an input action.yml does not declare: %s", st.Name, name, m[1])
		value, given := inputs[m[1]]
		if !given {
			value = input.Default
		}
		read[m[1]] = true
		env = append(env, name+"="+value)
	}
	for name := range inputs {
		gotest.True(t, read[name], "step %q does not read input %s", st.Name, name)
	}
	return env
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
func (s *ActionStepsTestSuite) step(t *gotest.T, key string, inputs map[string]string, env ...string) (actionRun, string) {
	return s.stepIn(t, t.TempDir(), key, inputs, env...)
}

// stepIn runs the action step key in dir with the given inputs, loggingCLI
// as the GOTEST_BIN the resolve step exports and coverGo as go, and returns
// the run and the CLI's one call, empty when it was not started.
func (s *ActionStepsTestSuite) stepIn(t *gotest.T, dir, key string, inputs map[string]string, env ...string) (actionRun, string) {
	action := readAction(t)
	st := actionStepNamed(t, action, key)
	tools := t.TempDir()
	cli, log := filepath.Join(tools, "gotest"), filepath.Join(tools, "calls")
	gotest.NoError(t, os.WriteFile(cli, []byte(loggingCLI), 0o700)) //nolint:gosec // G306: an executable
	vars := append(inputEnv(t, action, &st, inputs), "GOTEST_BIN="+cli, "STANDIN_LOG="+log, "GO_LOG="+filepath.Join(tools, "go.log"))
	run := runActionStep(t, st.Run, dir, map[string]string{"go": coverGo}, append(vars, env...)...)
	calls, _ := os.ReadFile(log)
	return run, strings.TrimSpace(string(calls))
}

func (s *ActionStepsTestSuite) test(t *gotest.T, inputs map[string]string, env ...string) (actionRun, string) {
	return s.step(t, "test", inputs, env...)
}

func (s *ActionStepsTestSuite) TestCoverageAddsOneProfileAndReportsItsTotal(t *gotest.T) {
	run, call := s.test(t, map[string]string{"coverage": "true"})
	profile := regexp.MustCompile(`--coverage=(\S+)`).FindStringSubmatch(call)
	gotest.Len(t, profile, 2, "call: %s", call)
	gotest.Contains(t, call, " -coverprofile="+profile[1])
	gotest.Contains(t, run.output, "coverage=81.5\n")
	_, err := os.Stat(profile[1])
	gotest.ErrorIs(t, err, fs.ErrNotExist, "the temporary profile is removed")
}

func (s *ActionStepsTestSuite) TestGoTestFlagsFollowThePackagesAndTheirProfileWins(t *gotest.T) {
	_, call := s.test(t, map[string]string{"coverage": "true", "race": "true", "go-test-flags": "-short\n-coverprofile=mine.out"})
	gotest.Regexp(t, `^summary --github --coverage=mine\.out \./\.\.\. -race -short -coverprofile=mine\.out$`, call)
}

func (s *ActionStepsTestSuite) TestGoTestFlagsWithQuotesAreRefused(t *gotest.T) {
	run, call := s.test(t, map[string]string{"go-test-flags": `-run "A B"`})
	gotest.Equal(t, 2, run.exit)
	gotest.Contains(t, run.stdout, "::error::go-test-flags must not contain quotes: one entry per word or line")
	gotest.Empty(t, call)
}

func (s *ActionStepsTestSuite) TestAVersionIsInstalled(t *gotest.T) {
	action := readAction(t)
	install := actionStepNamed(t, action, "Install gotest")
	gotest.Contains(t, install.If, "inputs.version != 'gomod'", "the install step runs only for a version tag")
	log := filepath.Join(t.TempDir(), "go.log")
	env := append(inputEnv(t, action, &install, map[string]string{"version": "v1.36.0"}), "GO_LOG="+log)
	run := runActionStep(t, install.Run, t.TempDir(), map[string]string{"go": coverGo}, env...)
	gotest.Equal(t, 0, run.exit, "stderr: %s", run.stderr)
	data, err := os.ReadFile(log)
	gotest.NoError(t, err)
	gotest.Equal(t, "install github.com/mvrahden/go-test/cmd/gotest@v1.36.0\n", string(data))
}

func (s *ActionStepsTestSuite) TestBenchInputsReachTheBenchCommand(t *gotest.T) {
	run, call := s.step(t, "bench", map[string]string{"flags": "-count=4",
		"bench-baseline": "base.json", "bench-gate": "10", "bench-save": "out.json"},
		`STANDIN_STDOUT={"gate":{"breachedKeys":["pkg.BenchmarkA","pkg.BenchmarkB"]}}`, "STANDIN_EXIT=1")
	gotest.Equal(t, "bench --spec --json --against=base.json --gate=10 --save=out.json -count=4 ./...", call)
	gotest.Equal(t, 1, run.exit)
	gotest.Contains(t, run.output, "bench-breached-keys=pkg.BenchmarkA,pkg.BenchmarkB\n")
	gotest.Regexp(t, `(?m)^bench-report=\S+$`, run.output)
}

func (s *ActionStepsTestSuite) TestBenchSaveDistinguishesOffFromTheConfiguredBaseline(t *gotest.T) {
	_, off := s.step(t, "bench", nil)
	gotest.Equal(t, "bench --spec --json ./...", off, "bench-save defaults to off")
	_, configured := s.step(t, "bench", map[string]string{"bench-save": ""})
	gotest.Equal(t, "bench --spec --json --save= ./...", configured)
}

func (s *ActionStepsTestSuite) TestFuzzReportsItsCrashersRelativeToTheCheckout(t *gotest.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	gotest.NoError(t, err)
	run, call := s.stepIn(t, dir, "fuzz", map[string]string{"fuzz-for": "30s"},
		"STANDIN_STDOUT=[FuzzParse] new crasher: "+filepath.Join(dir, "testdata", "fuzz", "FuzzParse", "abc"), "STANDIN_EXIT=1")
	gotest.Equal(t, "fuzz --for=30s ./...", call)
	gotest.Equal(t, 1, run.exit)
	gotest.Contains(t, run.output, "fuzz-crashers=testdata/fuzz/FuzzParse/abc\n")
	_, bare := s.step(t, "fuzz", nil)
	gotest.Equal(t, "fuzz ./...", bare, "fuzz-for defaults to the CLI's budget")
}

func (s *ActionStepsTestSuite) TestTheFuzzCacheStepsFollowTheirInput(t *gotest.T) {
	action := readAction(t)
	for _, name := range []string{"Locate the Go fuzz cache", "Restore the fuzz cache", "Save the fuzz cache"} {
		gotest.Contains(t, actionStepNamed(t, action, name).If, "inputs.fuzz-cache == 'true'", "step %q", name)
	}
}

// git runs git in dir with no user or system configuration and returns its
// trimmed output.
func (s *ActionStepsTestSuite) git(t *gotest.T, dir string, args ...string) string {
	cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...) //nolint:gosec // G204: git on the test's own repositories
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
	action := readAction(t)
	st := actionStepNamed(t, action, "Publish coverage badge")
	env := append(inputEnv(t, action, &st, map[string]string{"badge-branch": "metrics"}),
		"BADGE_FILE="+badge, "COVERAGE=81.5", "DEFAULT_BRANCH=main",
		"GH_TOKEN=tok", "GITHUB_REPOSITORY=o/r", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=url.file://"+bare+".insteadOf", "GIT_CONFIG_VALUE_0=https://x-access-token:tok@github.com/o/r.git")

	pr := runActionStep(t, st.Run, t.TempDir(), nil, append(env, "GITHUB_REF=refs/pull/7/merge")...)
	gotest.Contains(t, pr.stdout, "Coverage badge is published from main only; skipping on refs/pull/7/merge")
	gotest.Empty(t, s.git(t, bare, "branch", "--list", "metrics"))

	first := runActionStep(t, st.Run, t.TempDir(), nil, append(env, "GITHUB_REF=refs/heads/main")...)
	gotest.Equal(t, 0, first.exit, "stdout: %s\nstderr: %s", first.stdout, first.stderr)
	gotest.Contains(t, first.stdout, "Coverage badge published: https://raw.githubusercontent.com/o/r/metrics/coverage.svg")
	gotest.Equal(t, "<svg>81.5</svg>", s.git(t, bare, "show", "metrics:coverage.svg"))

	again := runActionStep(t, st.Run, t.TempDir(), nil, append(env, "GITHUB_REF=refs/heads/main")...)
	gotest.Contains(t, again.stdout, "Coverage badge unchanged (81.5%)")
}
