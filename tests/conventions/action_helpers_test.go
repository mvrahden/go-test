package conventions_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"go.yaml.in/yaml/v4"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// actionRun is what one run of an action.yml step did.
type actionRun struct {
	exit           int
	stdout, stderr string
	output, env    string // what the step appended to GITHUB_OUTPUT and GITHUB_ENV
	summary        string // what it appended to GITHUB_STEP_SUMMARY
	dir, temp      string // the working directory and RUNNER_TEMP
}

// actionStep returns the run script of action.yml's step with that id, or,
// when no step has it, that name.
func actionStep(t *gotest.T, key string) string {
	data, err := os.ReadFile(filepath.Join("..", "..", "action.yml"))
	gotest.NoError(t, err)
	var action struct {
		Runs struct {
			Steps []workflowStep `yaml:"steps"`
		} `yaml:"runs"`
	}
	gotest.NoError(t, yaml.Unmarshal(data, &action))
	byName := ""
	for i := range action.Runs.Steps {
		st := &action.Runs.Steps[i]
		if st.ID == key {
			return st.Run
		}
		if st.Name == key && byName == "" {
			byName = st.Run
		}
	}
	gotest.NotEmpty(t, byName, "action.yml has no step with id or name %q", key)
	return byName
}

// runActionStep runs script as GitHub runs a bash step, in dir (symlinks
// resolved, so /var and /private/var agree on macOS; PWD set to it), with
// bins (name to script) first on PATH, RUNNER_TEMP a fresh directory named
// "runner temp", RUNNER_OS=Linux, and env appended.
func runActionStep(t *gotest.T, script, dir string, bins map[string]string, env ...string) actionRun {
	dir, err := filepath.EvalSymlinks(dir)
	gotest.NoError(t, err)
	base, err := filepath.EvalSymlinks(t.TempDir())
	gotest.NoError(t, err)
	bin, temp := filepath.Join(base, "bin"), filepath.Join(base, "runner temp")
	gotest.NoError(t, os.MkdirAll(bin, 0o750))
	gotest.NoError(t, os.MkdirAll(temp, 0o750))
	for name, content := range bins {
		gotest.NoError(t, os.WriteFile(filepath.Join(bin, name), []byte(content), 0o700)) //nolint:gosec // G306: an executable
	}
	output, envFile := filepath.Join(base, "output"), filepath.Join(base, "env")
	cmd := exec.CommandContext(t.Context(), "bash", "--noprofile", "--norc", "-eo", "pipefail", "-c", script) //nolint:gosec // G204: the action's own script
	cmd.Dir = dir
	cmd.Env = append(stepEnviron(), "PWD="+dir,
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"RUNNER_TEMP="+temp, "RUNNER_OS=Linux",
		"GITHUB_OUTPUT="+output, "GITHUB_ENV="+envFile, "GITHUB_STEP_SUMMARY="+filepath.Join(base, "summary"))
	cmd.Env = append(cmd.Env, env...)
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	_ = cmd.Run()
	written, _ := os.ReadFile(output)
	envWritten, _ := os.ReadFile(envFile)
	summary, _ := os.ReadFile(filepath.Join(base, "summary"))
	return actionRun{
		exit:    cmd.ProcessState.ExitCode(),
		stdout:  stdout.String(),
		stderr:  stderr.String(),
		output:  string(written),
		env:     string(envWritten),
		summary: string(summary),
		dir:     dir,
		temp:    temp,
	}
}

// unixActionGuard is the SuiteGuard of every suite that runs steps: Windows
// and a missing bash skip.
func unixActionGuard() string {
	if runtime.GOOS == "windows" {
		return "the action's steps run under Git Bash there; the action-consumer job covers it"
	}
	if _, err := exec.LookPath("bash"); err != nil {
		return "bash not found"
	}
	return ""
}

// stepEnviron is this process's environment without what the Action passes
// its own steps: under CI these tests run inside it, and a step under test
// would otherwise read the outer run's inputs.
func stepEnviron() []string {
	var env []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "INPUT_") || strings.HasPrefix(kv, "GOTEST_BIN=") || strings.HasPrefix(kv, "GOTEST_CMD=") {
			continue
		}
		env = append(env, kv)
	}
	return env
}
