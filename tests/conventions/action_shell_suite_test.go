package conventions_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"go.yaml.in/yaml/v4"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// ActionShellTestSuite holds every run step of the composite actions to
// shellcheck, which actionlint does not reach inside action.yml files.
type ActionShellTestSuite struct{}

func (s *ActionShellTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second}
}

// A missing shellcheck skips the suite, except on a Linux runner, where it fails.
func (s *ActionShellTestSuite) SuiteGuard() string {
	if runtime.GOOS == "windows" {
		return "shellcheck is not on the Windows runners"
	}
	if _, err := exec.LookPath("shellcheck"); err != nil && (runtime.GOOS != "linux" || os.Getenv("GITHUB_ACTIONS") == "") {
		return "shellcheck not found"
	}
	return ""
}

// compositeStep is one run step of a composite action.
type compositeStep struct{ file, name, run string }

// compositeRunSteps reads action.yml and every .github/actions/*/action.yml
// and returns each step that has a run script, named by its name, else its
// id, else its index.
func compositeRunSteps(t *gotest.T) []compositeStep {
	root := filepath.Join("..", "..")
	files, err := filepath.Glob(filepath.Join(root, ".github", "actions", "*", "action.yml"))
	gotest.NoError(t, err)
	files = append([]string{filepath.Join(root, "action.yml")}, files...)
	var steps []compositeStep
	for _, file := range files {
		data, err := os.ReadFile(file)
		gotest.NoError(t, err)
		var action struct {
			Runs struct {
				Steps []struct {
					ID   string `yaml:"id"`
					Name string `yaml:"name"`
					Run  string `yaml:"run"`
				} `yaml:"steps"`
			} `yaml:"runs"`
		}
		gotest.NoError(t, yaml.Unmarshal(data, &action), "%s", file)
		rel, err := filepath.Rel(root, file)
		gotest.NoError(t, err)
		for i, step := range action.Runs.Steps {
			if step.Run == "" {
				continue
			}
			name := step.Name
			if name == "" {
				name = step.ID
			}
			if name == "" {
				name = strconv.Itoa(i)
			}
			steps = append(steps, compositeStep{file: filepath.ToSlash(rel), name: name, run: step.Run})
		}
	}
	return steps
}

func (s *ActionShellTestSuite) TestCompositeRunStepsPassShellcheck(t *gotest.T) {
	steps := compositeRunSteps(t)
	gotest.GreaterOrEqual(t, len(steps), 8, "action.yml and the local actions have at least eight run steps")
	var findings []string
	for _, step := range steps {
		cmd := exec.CommandContext(t.Context(), "shellcheck", "--shell=bash", "--severity=warning", "--format=gcc", "-")
		cmd.Stdin = strings.NewReader(step.run)
		out, err := cmd.CombinedOutput()
		// Exit 1 is findings; anything else means shellcheck did not judge the step.
		var exitErr *exec.ExitError
		if err != nil && (!errors.As(err, &exitErr) || exitErr.ExitCode() != 1) {
			t.Errorf("shellcheck did not run on %s: %s: %v\n%s", step.file, step.name, err, out)
			continue
		}
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if line != "" {
				findings = append(findings, step.file+": "+step.name+": "+strings.TrimPrefix(line, "-:"))
			}
		}
	}
	gotest.Empty(t, findings, "shellcheck findings in composite action steps")
}
