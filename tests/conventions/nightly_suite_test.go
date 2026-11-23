package conventions_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"go.yaml.in/yaml/v4"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// NightlyTestSuite runs the nightly's two scripts on recorded inputs and
// holds nightly.yml to the gate it repeats.
type NightlyTestSuite struct{}

func (s *NightlyTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second}
}

// A missing bash or jq skips the suite, except on a Linux runner, where it fails.
func (s *NightlyTestSuite) SuiteGuard() string {
	if runtime.GOOS == "windows" {
		return "the nightly's scripts run on Linux"
	}
	for _, tool := range []string{"bash", "jq"} {
		if _, err := exec.LookPath(tool); err != nil && (runtime.GOOS != "linux" || os.Getenv("GITHUB_ACTIONS") == "") {
			return tool + " not found"
		}
	}
	return ""
}

var scriptsDir = filepath.Join("..", "..", ".github", "scripts")

const events = `{"Action":"run","Package":"p","Test":"TestA"}
{"Action":"output","Package":"p","Test":"TestA","Output":"x\n"}
{"Action":"%s","Package":"p","Test":"TestA"}
{"Action":"pass","Package":"p","Test":"TestB"}
{"Action":"pass","Package":"p"}
`

// mixed runs mixed-verdicts.sh on one file per stream and returns its
// stdout and exit code.
func (s *NightlyTestSuite) mixed(t *gotest.T, streams ...string) (string, int) {
	dir := t.TempDir()
	args := []string{filepath.Join(scriptsDir, "mixed-verdicts.sh")}
	for i, stream := range streams {
		path := filepath.Join(dir, fmt.Sprintf("events-%d.json", i+1))
		gotest.NoError(t, os.WriteFile(path, []byte(stream), 0o600))
		args = append(args, path)
	}
	cmd := exec.CommandContext(t.Context(), "bash", args...)
	cmd.Env = slices.DeleteFunc(os.Environ(), func(kv string) bool { return strings.HasPrefix(kv, "GITHUB_STEP_SUMMARY=") })
	out, _ := cmd.Output()
	return string(out), cmd.ProcessState.ExitCode()
}

// ghStandIn logs its arguments and answers issue list with OPEN_ISSUE.
const ghStandIn = `#!/usr/bin/env bash
echo "$*" >> "$GH_LOG"
if [ "$1 $2" = "issue list" ]; then printf '%s\n' "$OPEN_ISSUE"; fi
`

// report runs nightly-report.sh on needs with a stand-in gh, and returns
// the gh calls (one per line that starts "issue ") and stdout.
func (s *NightlyTestSuite) report(t *gotest.T, needs, open string) ([]string, string) {
	dir := t.TempDir()
	file, log := filepath.Join(dir, "needs.json"), filepath.Join(dir, "gh.log")
	gotest.NoError(t, os.WriteFile(file, []byte(needs), 0o600))
	bin := filepath.Join(dir, "bin")
	gotest.NoError(t, os.MkdirAll(bin, 0o750))
	gotest.NoError(t, os.WriteFile(filepath.Join(bin, "gh"), []byte(ghStandIn), 0o700)) //nolint:gosec // G306: an executable

	cmd := exec.CommandContext(t.Context(), "bash", filepath.Join(scriptsDir, "nightly-report.sh"), file, "https://example.test/run/1") //nolint:gosec // G204: the repository's own script
	cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "GH_LOG="+log, "OPEN_ISSUE="+open)
	out, err := cmd.Output()
	gotest.NoError(t, err, "stdout: %s", out)
	data, _ := os.ReadFile(log)
	var calls []string
	for _, line := range strings.SplitAfter(string(data), "\n") {
		if strings.HasPrefix(line, "issue ") || len(calls) == 0 {
			calls = append(calls, line)
		} else {
			calls[len(calls)-1] += line
		}
	}
	for i := range calls {
		calls[i] = strings.TrimSuffix(calls[i], "\n")
	}
	if len(calls) == 1 && calls[0] == "" {
		calls = nil
	}
	return calls, string(out)
}

func (s *NightlyTestSuite) TestRunsThatAgreeReportNothing(t *gotest.T) {
	out, code := s.mixed(t, fmt.Sprintf(events, "pass"), fmt.Sprintf(events, "pass"), fmt.Sprintf(events, "pass"))
	gotest.Equal(t, 0, code)
	gotest.Equal(t, "No test changed its verdict across 3 runs.\n", out)
}

func (s *NightlyTestSuite) TestATestThatPassedAndFailedIsListed(t *gotest.T) {
	out, code := s.mixed(t, fmt.Sprintf(events, "pass"), fmt.Sprintf(events, "fail"), fmt.Sprintf(events, "pass"))
	gotest.Equal(t, 1, code)
	gotest.Equal(t, "1 tests changed their verdict across 3 runs:\n\n| Test | Run 1 | Run 2 | Run 3 |\n|---|---|---|---|\n| p TestA | pass | fail | pass |\n", out)
}

func (s *NightlyTestSuite) TestATestMissingFromARunIsListed(t *gotest.T) {
	out, code := s.mixed(t, fmt.Sprintf(events, "pass"), `{"Action":"pass","Package":"p","Test":"TestB"}`+"\n")
	gotest.Equal(t, 1, code)
	gotest.Contains(t, out, "| p TestA | pass | none |\n")
	gotest.NotContains(t, out, "TestB")
}

func (s *NightlyTestSuite) TestOneStreamIsNotAComparison(t *gotest.T) {
	out, code := s.mixed(t, fmt.Sprintf(events, "pass"))
	gotest.Equal(t, 2, code)
	gotest.Contains(t, out, "usage: mixed-verdicts.sh <events file> <events file>...")
}

func (s *NightlyTestSuite) TestAGreenNightReportsNothing(t *gotest.T) {
	calls, out := s.report(t, `{"gate":{"result":"success","outputs":{}},"go-tip":{"result":"success","outputs":{"outcome":"success"}}}`, "")
	gotest.Empty(t, calls)
	gotest.Equal(t, "nightly: nothing to report\n", out)
}

func (s *NightlyTestSuite) TestARedNightOpensOneIssueAndThenCommentsOnIt(t *gotest.T) {
	needs := `{"gate":{"result":"failure","outputs":{}},"drill":{"result":"cancelled","outputs":{}},"checks":{"result":"success","outputs":{}},"go-tip":{"result":"success","outputs":{"outcome":"failure"}}}`
	calls, _ := s.report(t, needs, "")
	gotest.Len(t, calls, 2)
	gotest.Equal(t, `issue create --title Nightly is red --body Run: https://example.test/run/1`+"\n"+`Failed: drill, gate`+"\n"+`Go tip: failed`, calls[1])
	calls, _ = s.report(t, needs, "12")
	gotest.Len(t, calls, 2)
	gotest.Regexp(t, `^issue comment 12 --body Run: `, calls[1])
}

func (s *NightlyTestSuite) TestAGoTipFailureAloneHasItsOwnIssue(t *gotest.T) {
	calls, _ := s.report(t, `{"gate":{"result":"success","outputs":{}},"go-tip":{"result":"success","outputs":{"outcome":"failure"}}}`, "")
	gotest.Len(t, calls, 2)
	gotest.Regexp(t, `^issue create --title Nightly: Go tip is red --body Run: https://example.test/run/1`+"\n"+`Go tip: failed$`, calls[1])
}

// actionStepOf is the job's step that runs the action.
func actionStepOf(t *gotest.T, job *workflowJob) workflowStep {
	at := slices.IndexFunc(job.Steps, func(st workflowStep) bool { return st.Uses == "./" })
	gotest.GreaterOrEqual(t, at, 0, "the job has no step that runs the action")
	return job.Steps[at]
}

// runOf is the run script of the job's step with that name.
func runOf(t *gotest.T, job *workflowJob, name string) string {
	at := slices.IndexFunc(job.Steps, func(st workflowStep) bool { return st.Name == name })
	gotest.GreaterOrEqual(t, at, 0, "the job has no step %q", name)
	return job.Steps[at].Run
}

// without is a copy of with without keys.
func without(with map[string]string, keys ...string) map[string]string {
	out := map[string]string{}
	for k, v := range with {
		if !slices.Contains(keys, k) {
			out[k] = v
		}
	}
	return out
}

func (s *NightlyTestSuite) TestTheNightlyRunsTheGateAsPushesDo(t *gotest.T) {
	nightly, test, quality := readWorkflow(t, "nightly.yml"), readWorkflow(t, "test.yml"), readWorkflow(t, "quality.yml")
	testJob, gateJob := test.Jobs["test"], nightly.Jobs["gate"]
	platformTest, platform := test.Jobs["platform-test"], nightly.Jobs["platform"]
	qualityDrill, nightlyDrill := quality.Jobs["drill"], nightly.Jobs["drill"]
	gotest.Equal(t, without(actionStepOf(t, &testJob).With, "badge", "events-file"), without(actionStepOf(t, &gateJob).With, "events-file"))
	gotest.Equal(t, actionStepOf(t, &platformTest).With, actionStepOf(t, &platform).With)
	gotest.Equal(t, actionStepOf(t, &platformTest).Env, actionStepOf(t, &platform).Env)
	gotest.Equal(t, runOf(t, &qualityDrill, "Drill"), runOf(t, &nightlyDrill, "Drill"))
	gotest.Contains(t, eventPaths(t, nightly, "pull_request"), ".github/workflows/nightly.yml")
	_, scheduled := nightly.On["schedule"]
	gotest.True(t, scheduled)
}

func (s *NightlyTestSuite) TestGoTipCannotFailTheRunAndIsStillReported(t *gotest.T) {
	nightly := readWorkflow(t, "nightly.yml")
	tip := nightly.Jobs["go-tip"]
	gotest.Zero(t, tip.ContinueOnErr.Value, "a job-level continue-on-error hides the outcome from the report")
	at := slices.IndexFunc(tip.Steps, func(st workflowStep) bool { return st.ID == "install" })
	gotest.GreaterOrEqual(t, at, 0, "go-tip has no install step")
	for i := at; i < len(tip.Steps); i++ {
		st := &tip.Steps[i]
		gotest.Equal(t, "true", st.ContinueOnErr.Value, "go-tip step %q can fail the run", st.Name)
	}
	data, err := os.ReadFile(filepath.Join(workflowsDir, "nightly.yml"))
	gotest.NoError(t, err)
	gotest.Contains(t, string(data), "outcome: ${{ steps.")
	var jobs struct {
		Jobs map[string]struct {
			Needs yaml.Node `yaml:"needs"` // a string or a list
		} `yaml:"jobs"`
	}
	gotest.NoError(t, yaml.Unmarshal(data, &jobs))
	var needs []string
	report := jobs.Jobs["report"].Needs
	gotest.NoError(t, report.Decode(&needs))
	gotest.Contains(t, needs, "go-tip")
}

func (s *NightlyTestSuite) TestTheNightlyHasNoCommentLines(t *gotest.T) {
	data, err := os.ReadFile(filepath.Join(workflowsDir, "nightly.yml"))
	gotest.NoError(t, err)
	var comments []string
	for i, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			comments = append(comments, fmt.Sprintf("nightly.yml:%d", i+1))
		}
	}
	gotest.Empty(t, comments)
}
