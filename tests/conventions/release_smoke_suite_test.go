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

// ReleaseSmokeTestSuite runs the release workflow's smoke step with a
// stand-in binary: a binary that reports another version, or that fails its
// run, stops the publish job before the upload.
type ReleaseSmokeTestSuite struct {
	script string
	jobs   map[string]releaseJob // read-only after BeforeAll
}

type releaseJob struct {
	Needs any `yaml:"needs"` // a string or a list
	Steps []struct {
		ID   string `yaml:"id"`
		Name string `yaml:"name"`
		If   string `yaml:"if"`
		Run  string `yaml:"run"`
		Uses string `yaml:"uses"`
	} `yaml:"steps"`
}

func (s *ReleaseSmokeTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second}
}

func (s *ReleaseSmokeTestSuite) SuiteGuard() string {
	if runtime.GOOS == "windows" {
		return "the stand-in binary is a shell script"
	}
	if _, err := exec.LookPath("bash"); err != nil {
		return "bash not found"
	}
	return ""
}

func (s *ReleaseSmokeTestSuite) BeforeAll(t *gotest.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "release.yml"))
	gotest.NoError(t, err)
	var workflow struct {
		Jobs map[string]releaseJob `yaml:"jobs"`
	}
	gotest.NoError(t, yaml.Unmarshal(data, &workflow))
	s.jobs = workflow.Jobs
	if i := s.stepIndex("publish", func(id, _ string) bool { return id == "smoke" }); i >= 0 {
		s.script = s.jobs["publish"].Steps[i].Run
	}
	gotest.NotEmpty(t, s.script, "release.yml's publish job has no step with id smoke")
}

// AfterAll has nothing to release: every run lives in a temp dir.
func (s *ReleaseSmokeTestSuite) AfterAll(t *gotest.T) {}

// stepIndex is the index of the first step of job that match takes, or -1.
func (s *ReleaseSmokeTestSuite) stepIndex(job string, match func(id, name string) bool) int {
	for i, step := range s.jobs[job].Steps {
		if match(step.ID, step.Name) {
			return i
		}
	}
	return -1
}

// needsOf lists the jobs job waits for.
func (s *ReleaseSmokeTestSuite) needsOf(job string) []string {
	switch v := s.jobs[job].Needs.(type) {
	case string:
		return []string{v}
	case []any:
		out := make([]string, 0, len(v))
		for _, n := range v {
			name, _ := n.(string)
			out = append(out, name)
		}
		return out
	}
	return nil
}

// smokeStandIn logs where and how it is called, and answers version with
// STANDIN_VERSION.
const smokeStandIn = `#!/usr/bin/env bash
echo "$PWD|$*" >> "$STANDIN_LOG"
if [ "$1" = version ]; then echo "$STANDIN_VERSION"; exit 0; fi
exit "${STANDIN_EXIT:-0}"
`

const taggedVersion = "gotest v1.38.0 (abc1234) go1.27.3 linux/amd64"

type smokeRun struct {
	exit   int
	stdout string
	calls  []string
}

// smoke runs the step in a fresh directory holding the stand-in, as an
// artifact download leaves it: without the executable bit.
func (s *ReleaseSmokeTestSuite) smoke(t *gotest.T, version, tag, exit string) smokeRun {
	dir := t.TempDir()
	gotest.NoError(t, os.MkdirAll(filepath.Join(dir, "dist"), 0o750))
	gotest.NoError(t, os.MkdirAll(filepath.Join(dir, "examples"), 0o750))
	gotest.NoError(t, os.WriteFile(filepath.Join(dir, "dist", "gotest-linux-amd64"), []byte(smokeStandIn), 0o600))
	log := filepath.Join(t.TempDir(), "calls")
	cmd := exec.CommandContext(t.Context(), "bash", "--noprofile", "--norc", "-eo", "pipefail", "-c", s.script) //nolint:gosec // G204: the workflow's own script
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "PWD="+dir, "TAG="+tag, "STANDIN_LOG="+log,
		"STANDIN_VERSION="+version, "STANDIN_EXIT="+exit)
	out, _ := cmd.Output()
	calls, _ := os.ReadFile(log)
	return smokeRun{
		exit:   cmd.ProcessState.ExitCode(),
		stdout: string(out),
		calls:  strings.Split(strings.TrimSpace(string(calls)), "\n"),
	}
}

func (s *ReleaseSmokeTestSuite) TestTheTaggedBinaryRunsTheExample(t *gotest.T) {
	run := s.smoke(t, taggedVersion, "v1.38.0", "")
	gotest.Equal(t, 0, run.exit, "stdout: %s", run.stdout)
	gotest.Len(t, run.calls, 2, "stdout: %s", run.stdout)
	gotest.Regexp(t, `\|version$`, run.calls[0])
	gotest.Regexp(t, `/examples\|\./cart/\.\.\.$`, run.calls[1])
}

func (s *ReleaseSmokeTestSuite) TestABinaryOfAnotherVersionStopsTheBuild(t *gotest.T) {
	run := s.smoke(t, taggedVersion, "v1.38.1", "")
	gotest.Equal(t, 1, run.exit, "stdout: %s", run.stdout)
	gotest.Contains(t, run.stdout, "::error::gotest-linux-amd64 reports '"+taggedVersion+"', not v1.38.1")
	gotest.Len(t, run.calls, 1)
}

func (s *ReleaseSmokeTestSuite) TestAnUnstampedBinaryStopsTheBuild(t *gotest.T) {
	run := s.smoke(t, "gotest dev (source checkout) go1.27.3 linux/amd64", "v1.38.0", "")
	gotest.Equal(t, 1, run.exit, "stdout: %s", run.stdout)
	gotest.Contains(t, run.stdout, "not v1.38.0")
	gotest.Len(t, run.calls, 1)
}

func (s *ReleaseSmokeTestSuite) TestALongerVersionIsNotTheTag(t *gotest.T) {
	run := s.smoke(t, "gotest v1.38.01 (abc1234) go1.27.3 linux/amd64", "v1.38.0", "")
	gotest.Equal(t, 1, run.exit, "stdout: %s", run.stdout)
	gotest.Len(t, run.calls, 1)
}

func (s *ReleaseSmokeTestSuite) TestAFailingRunStopsTheBuild(t *gotest.T) {
	run := s.smoke(t, taggedVersion, "v1.38.0", "2")
	gotest.Equal(t, 2, run.exit, "stdout: %s", run.stdout)
	gotest.Len(t, run.calls, 2)
}

func (s *ReleaseSmokeTestSuite) TestTheSmokeRunComesBeforeTheUpload(t *gotest.T) {
	smoke := s.stepIndex("publish", func(id, _ string) bool { return id == "smoke" })
	upload := s.stepIndex("publish", func(_, name string) bool { return name == "Upload to release" })
	gotest.GreaterOrEqual(t, smoke, 0)
	gotest.Less(t, smoke, upload)
}

func (s *ReleaseSmokeTestSuite) TestOnlyPublishUploadsToTheRelease(t *gotest.T) {
	var uploads []string
	for name, job := range s.jobs {
		for _, step := range job.Steps {
			if name != "publish" && strings.Contains(step.Run, "gh release upload") {
				uploads = append(uploads, name+": "+step.Name)
			}
		}
	}
	gotest.Empty(t, uploads)
}

func (s *ReleaseSmokeTestSuite) TestNothingIsPublishedPastAFailedSmokeRun(t *gotest.T) {
	gotest.Equal(t, []string{"publish"}, s.needsOf("update-major-tag"))
	gotest.Equal(t, []string{"publish"}, s.needsOf("extension"))
	gotest.Equal(t, []string{"build"}, s.needsOf("publish"))
}
