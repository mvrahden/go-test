package conventions_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// ActionEventsFileTestSuite runs the action's test step with a stand-in CLI:
// the events-file input adds the flag and sets the output, and a CLI too old
// for the flag fails the step before anything runs.
type ActionEventsFileTestSuite struct {
	script string
}

func (s *ActionEventsFileTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second}
}

func (s *ActionEventsFileTestSuite) SuiteGuard() string {
	if runtime.GOOS == "windows" {
		return "the stand-in CLI is a shell script"
	}
	if _, err := exec.LookPath("bash"); err != nil {
		return "bash not found"
	}
	return ""
}

func (s *ActionEventsFileTestSuite) BeforeAll(t *gotest.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "action.yml"))
	gotest.NoError(t, err)
	var action struct {
		Runs struct {
			Steps []struct {
				ID  string `yaml:"id"`
				Run string `yaml:"run"`
			} `yaml:"steps"`
		} `yaml:"runs"`
	}
	gotest.NoError(t, yaml.Unmarshal(data, &action))
	for _, step := range action.Runs.Steps {
		if step.ID == "test" {
			s.script = step.Run
		}
	}
	gotest.NotEmpty(t, s.script, "action.yml has no step with id test")
}

// AfterAll has nothing to release: every run lives in a temp dir.
func (s *ActionEventsFileTestSuite) AfterAll(t *gotest.T) {}

// standIn prints help naming --events-file unless old, writes "{}" to the
// file the flag names, and logs every call.
const standIn = `#!/usr/bin/env bash
echo "$*" >> "$STANDIN_LOG"
for a in "$@"; do
  case "$a" in
    --help)
      echo "Flags:"
      [ -z "$STANDIN_OLD" ] && echo "  --events-file=<file>"
      exit 0 ;;
    --events-file=*)
      f="${a#--events-file=}"; mkdir -p "$(dirname "$f")"; echo '{}' > "$f" ;;
  esac
done
`

type stepRun struct {
	exit           int
	stdout, output string
	calls          []string
	dir            string
}

// runStep runs the test step as GitHub runs a bash step, in a fresh working
// directory, with the stand-in as gotest.
func (s *ActionEventsFileTestSuite) runStep(t *gotest.T, eventsFile string, old bool, extra ...string) stepRun {
	bin, dir := t.TempDir(), t.TempDir()
	gotest.NoError(t, os.WriteFile(filepath.Join(bin, "gotest"), []byte(standIn), 0o700)) //nolint:gosec // G306: an executable
	for i := 0; i+1 < len(extra); i += 2 {
		gotest.NoError(t, os.WriteFile(filepath.Join(bin, extra[i]), []byte(extra[i+1]), 0o700)) //nolint:gosec // G306: an executable
	}
	log, output := filepath.Join(bin, "calls"), filepath.Join(bin, "output")
	cmd := exec.CommandContext(t.Context(), "bash", "--noprofile", "--norc", "-eo", "pipefail", "-c", s.script) //nolint:gosec // G204: the action's own script
	cmd.Dir = dir
	// PWD as a runner's shell has it: inherited, bash would distrust it and
	// read the resolved path, /private/var/... on macOS.
	cmd.Env = append(os.Environ(), "PWD="+dir,
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"GITHUB_OUTPUT="+output, "RUNNER_TEMP="+bin, "STANDIN_LOG="+log,
		"INPUT_VERSION=v1.33.0", "INPUT_PACKAGES=./...", "INPUT_EVENTS_FILE="+eventsFile)
	if old {
		cmd.Env = append(cmd.Env, "STANDIN_OLD=1")
	}
	out, _ := cmd.Output()
	calls, _ := os.ReadFile(log)
	written, _ := os.ReadFile(output)
	return stepRun{
		exit:   cmd.ProcessState.ExitCode(),
		stdout: string(out),
		output: string(written),
		calls:  strings.Split(strings.TrimSpace(string(calls)), "\n"),
		dir:    dir,
	}
}

func (s *ActionEventsFileTestSuite) TestUnsetAddsNothing(t *gotest.T) {
	run := s.runStep(t, "", false)
	gotest.Equal(t, 0, run.exit, "stdout: %s", run.stdout)
	gotest.Len(t, run.calls, 1)
	gotest.NotContains(t, run.calls[0], "--events-file")
	gotest.NotContains(t, run.output, "events-file=")
}

func (s *ActionEventsFileTestSuite) TestSetPassesTheFlagAndReturnsTheAbsolutePath(t *gotest.T) {
	run := s.runStep(t, "out/events.json", false)
	want := filepath.Join(run.dir, "out", "events.json")
	gotest.Equal(t, 0, run.exit, "stdout: %s", run.stdout)
	gotest.Contains(t, run.calls[len(run.calls)-1], "--events-file="+want)
	gotest.Contains(t, run.output, "events-file="+want+"\n")
	_, err := os.Stat(want)
	gotest.NoError(t, err)
}

// cygpathStandIn prints the path it is given as Git Bash's cygpath -m would
// print a Windows one: drive letter first.
const cygpathStandIn = `#!/usr/bin/env bash
echo "W:$2"
`

func (s *ActionEventsFileTestSuite) TestUnderGitBashTheOutputIsAWindowsPath(t *gotest.T) {
	run := s.runStep(t, "out/events.json", false, "cygpath", cygpathStandIn)
	want := "W:" + filepath.Join(run.dir, "out", "events.json")
	gotest.Equal(t, 0, run.exit, "stdout: %s", run.stdout)
	gotest.Contains(t, run.calls[len(run.calls)-1], "--events-file="+want)
	gotest.Contains(t, run.output, "events-file="+want+"\n")
}

func (s *ActionEventsFileTestSuite) TestAnOlderCLIFailsBeforeRunning(t *gotest.T) {
	run := s.runStep(t, "out/events.json", true)
	gotest.Equal(t, 2, run.exit, "stdout: %s", run.stdout)
	gotest.Contains(t, run.stdout, "::error::events-file needs gotest v1.36.0 or newer; the version input is v1.33.0")
	gotest.Len(t, run.calls, 1)
	gotest.Contains(t, run.calls[0], "--help")
	gotest.NotContains(t, run.output, "events-file=")
}
