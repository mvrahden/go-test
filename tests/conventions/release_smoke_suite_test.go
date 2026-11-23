package conventions_test

import (
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

// ReleaseSmokeTestSuite runs the release workflow's smoke step with a
// stand-in binary: a binary that reports another version, or that fails its
// run, stops the publish job before the upload.
type ReleaseSmokeTestSuite struct {
	script    string
	pluginTag string                // the plugin tag step's script
	jobs      map[string]releaseJob // read-only after BeforeAll
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
	if i := s.stepIndex("tag-lint-plugin", func(id, _ string) bool { return id == "plugin-tag" }); i >= 0 {
		s.pluginTag = s.jobs["tag-lint-plugin"].Steps[i].Run
	}
	gotest.NotEmpty(t, s.pluginTag, "release.yml's tag-lint-plugin job has no step with id plugin-tag")
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

// pluginRepo is a clone whose origin is a bare repository beside it, with
// one commit; withPlugin puts the golangci plugin's go.mod in it.
type pluginRepo struct {
	dir, origin, sha string
	env              []string
}

func newPluginRepo(t *gotest.T, withPlugin bool) *pluginRepo {
	root := t.TempDir()
	r := &pluginRepo{dir: filepath.Join(root, "work"), origin: filepath.Join(root, "origin.git")}
	r.env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid")
	r.git(t, root, "init", "-q", "--bare", r.origin)
	r.git(t, root, "init", "-q", r.dir)
	path := filepath.Join(r.dir, "README")
	if withPlugin {
		path = filepath.Join(r.dir, "pkg", "lint", "golangci", "go.mod")
		gotest.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	}
	gotest.NoError(t, os.WriteFile(path, []byte("module example.com/plugin\n"), 0o600))
	r.git(t, r.dir, "add", "-A")
	r.git(t, r.dir, "commit", "-q", "-m", "one")
	r.git(t, r.dir, "remote", "add", "origin", r.origin)
	r.sha = strings.TrimSpace(r.git(t, r.dir, "rev-parse", "HEAD"))
	return r
}

func (r *pluginRepo) git(t *gotest.T, dir string, args ...string) string {
	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir, cmd.Env = dir, r.env
	out, err := cmd.CombinedOutput()
	gotest.NoError(t, err, string(out))
	return string(out)
}

// originTag is what origin's tag name points at: the object, then the commit.
func (r *pluginRepo) originTag(t *gotest.T, name string) (object, commit string) {
	resolve := func(rev string) string {
		cmd := exec.CommandContext(t.Context(), "git", "--git-dir", r.origin, "rev-parse", "--verify", "--quiet", rev) //nolint:gosec // G204: the test's own repository
		cmd.Env = r.env
		out, _ := cmd.Output()
		return strings.TrimSpace(string(out))
	}
	return resolve("refs/tags/" + name), resolve("refs/tags/" + name + "^{commit}")
}

func (s *ReleaseSmokeTestSuite) tagPlugin(t *gotest.T, r *pluginRepo, tag string) smokeRun {
	cmd := exec.CommandContext(t.Context(), "bash", "--noprofile", "--norc", "-eo", "pipefail", "-c", s.pluginTag) //nolint:gosec // G204: the workflow's own script
	cmd.Dir = r.dir
	cmd.Env = append(slices.Clip(r.env), "TAG="+tag, "GITHUB_SHA="+r.sha)
	out, _ := cmd.CombinedOutput()
	return smokeRun{exit: cmd.ProcessState.ExitCode(), stdout: string(out)}
}

func (s *ReleaseSmokeTestSuite) TestTheReleaseTagsThePluginModule(t *gotest.T) {
	r := newPluginRepo(t, true)
	run := s.tagPlugin(t, r, "v1.39.0")
	object, commit := r.originTag(t, "pkg/lint/golangci/v1.39.0")
	t.When("the release commit has the golangci plugin module and origin has no tag for it", func(w *gotest.T) {
		w.It("pushes pkg/lint/golangci/<tag> at the release commit, a lightweight tag", func(it *gotest.T) {
			gotest.Equal(it, 0, run.exit, run.stdout)
			gotest.Equal(it, r.sha, commit, run.stdout)
			gotest.Equal(it, r.sha, object, "the tag is an annotated tag object")
		})
	})
}

func (s *ReleaseSmokeTestSuite) TestAPluginTagAtTheReleaseCommitIsKept(t *gotest.T) {
	r := newPluginRepo(t, true)
	r.git(t, r.dir, "tag", "pkg/lint/golangci/v1.39.0")
	r.git(t, r.dir, "push", "-q", "origin", "refs/tags/pkg/lint/golangci/v1.39.0")
	run := s.tagPlugin(t, r, "v1.39.0")
	gotest.Equal(t, 0, run.exit, run.stdout)
	gotest.Contains(t, run.stdout, "already points at "+r.sha)
}

func (s *ReleaseSmokeTestSuite) TestAPluginTagAtAnotherCommitFailsTheJob(t *gotest.T) {
	r := newPluginRepo(t, true)
	r.git(t, r.dir, "tag", "pkg/lint/golangci/v1.39.0")
	r.git(t, r.dir, "push", "-q", "origin", "refs/tags/pkg/lint/golangci/v1.39.0")
	r.git(t, r.dir, "commit", "-q", "--allow-empty", "-m", "two")
	r.sha = strings.TrimSpace(r.git(t, r.dir, "rev-parse", "HEAD"))
	run := s.tagPlugin(t, r, "v1.39.0")
	_, commit := r.originTag(t, "pkg/lint/golangci/v1.39.0")
	gotest.Equal(t, 1, run.exit, run.stdout)
	gotest.Contains(t, run.stdout, "::error::pkg/lint/golangci/v1.39.0 points at ")
	gotest.NotEqual(t, r.sha, commit, "the tag was moved")
}

func (s *ReleaseSmokeTestSuite) TestACommitWithoutThePluginGetsNoPluginTag(t *gotest.T) {
	r := newPluginRepo(t, false)
	run := s.tagPlugin(t, r, "v1.39.0")
	object, _ := r.originTag(t, "pkg/lint/golangci/v1.39.0")
	gotest.Equal(t, 0, run.exit, run.stdout)
	gotest.Empty(t, object)
}

func (s *ReleaseSmokeTestSuite) TestThePluginIsTaggedOnceThePreflightPassed(t *gotest.T) {
	gotest.Equal(t, []string{"preflight"}, s.needsOf("tag-lint-plugin"))
}
