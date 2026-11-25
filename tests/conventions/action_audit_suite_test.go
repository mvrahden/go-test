package conventions_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// ActionAuditTestSuite runs the action's test, audit and predicate steps
// under a stand-in CLI: on a pull request the run keeps its stream and
// profile, the audit compares with the base commit, fetched when the
// checkout lacks it, and the predicate's subject is the tested commit.
type ActionAuditTestSuite struct{}

func (s *ActionAuditTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second}
}

func (s *ActionAuditTestSuite) SuiteGuard() string {
	if reason := unixActionGuard(); reason != "" {
		return reason
	}
	if _, err := exec.LookPath("git"); err != nil {
		return "git not found"
	}
	return ""
}

// auditCLI logs each call, knows --events-file, --evidence and, unless
// STANDIN_NO_MUTATE, --mutate-budget, writes the
// files --events-file, -coverprofile and --output name, and exits
// STANDIN_AUDIT_EXIT from audit. Its help runs on past a pipe's buffer, so
// a probe that stops reading at the flag breaks the pipe it writes to.
const auditCLI = `#!/usr/bin/env bash
echo "$*" >> "$STANDIN_LOG"
out=""
for a in "$@"; do
  case "$a" in
    --help) echo "  --events-file=<file>"; echo "  --evidence=<stream>"; [ -z "$STANDIN_NO_MUTATE" ] && echo "  --mutate-budget=<d>"; printf '%1048576s\n' ''; exit 0 ;;
    --events-file=*) echo '{}' > "${a#--events-file=}" ;;
    -coverprofile=*) echo 'mode: set' > "${a#-coverprofile=}" ;;
    --output=*) out="${a#--output=}" ;;
  esac
done
case "$1" in
  audit) [ -n "$out" ] && echo "## gotest audit from the stand-in" > "$out"; exit "${STANDIN_AUDIT_EXIT:-0}" ;;
  summary) [ -n "$out" ] && echo '{"result":"PASSED"}' > "$out" ;;
esac
exit 0
`

// auditRun is one run of a step and the CLI calls it made.
type auditRun struct {
	actionRun
	calls []string
}

// call is the logged call that starts with prefix, "" for none.
func (r *auditRun) call(prefix string) string {
	for _, c := range r.calls {
		if strings.HasPrefix(c, prefix) {
			return c
		}
	}
	return ""
}

// runStep runs the action's step key in dir with the inputs, the stand-in
// as GOTEST_BIN and coverGo as go, plus env.
func (s *ActionAuditTestSuite) runStep(t *gotest.T, dir, key string, inputs map[string]string, env ...string) auditRun {
	action := readAction(t)
	st := actionStepNamed(t, action, key)
	tools := t.TempDir()
	cli, log := filepath.Join(tools, "gotest"), filepath.Join(tools, "calls")
	gotest.NoError(t, os.WriteFile(cli, []byte(auditCLI), 0o700)) //nolint:gosec // G306: an executable
	vars := append(inputEnv(t, action, &st, inputs), "GOTEST_BIN="+cli, "STANDIN_LOG="+log, "GO_LOG="+filepath.Join(tools, "go.log"),
		"GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1", "GIT_CEILING_DIRECTORIES="+filepath.Dir(dir))
	run := runActionStep(t, st.Run, dir, map[string]string{"go": coverGo}, append(vars, env...)...)
	data, _ := os.ReadFile(log)
	var calls []string
	if text := strings.TrimSpace(string(data)); text != "" {
		calls = strings.Split(text, "\n")
	}
	return auditRun{actionRun: run, calls: calls}
}

// auditGit runs git in dir with no user or system configuration and
// returns its trimmed output.
func auditGit(t *gotest.T, dir string, args ...string) string {
	cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...) //nolint:gosec // G204: git on the test's own repositories
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	gotest.NoError(t, err, "git %v: %s", args, out)
	return strings.TrimSpace(string(out))
}

// twoCommits is a repository with a base commit and a head commit on it;
// it returns the directory and both commits.
func twoCommits(t *gotest.T) (dir, base, head string) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	gotest.NoError(t, err)
	commit := []string{"-c", "user.name=gotest", "-c", "user.email=gotest@example.com", "commit", "-q", "-a", "-m"}
	auditGit(t, dir, "init", "-q")
	gotest.NoError(t, os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n"), 0o600))
	auditGit(t, dir, "add", "-A")
	auditGit(t, dir, append(commit, "base")...)
	base = auditGit(t, dir, "rev-parse", "HEAD")
	gotest.NoError(t, os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n\nvar x = 1\n"), 0o600))
	auditGit(t, dir, append(commit, "head")...)
	return dir, base, auditGit(t, dir, "rev-parse", "HEAD")
}

func (s *ActionAuditTestSuite) TestAPullRequestRunKeepsItsStreamAndProfile(t *gotest.T) {
	run := s.runStep(t, t.TempDir(), "test", map[string]string{"coverage": "true"}, "EVENT_NAME=pull_request")
	gotest.Equal(t, 0, run.exit, "stdout: %s", run.stdout)
	stream, profile := filepath.Join(run.temp, "gotest-run-events.json"), filepath.Join(run.temp, "gotest-cover.out")
	call := run.call("summary --github")
	gotest.Contains(t, call, " --events-file="+stream+" ")
	gotest.Contains(t, call, " -coverprofile="+profile)
	gotest.Contains(t, run.output, "stream="+stream+"\n")
	gotest.Contains(t, run.output, "profile="+profile+"\n")
	gotest.NotContains(t, run.output, "events-file=", "the events-file output stays the caller's")
}

func (s *ActionAuditTestSuite) TestAPushRunKeepsNothingExtra(t *gotest.T) {
	run := s.runStep(t, t.TempDir(), "test", map[string]string{"coverage": "true"}, "EVENT_NAME=push")
	gotest.Equal(t, 0, run.exit, "stdout: %s", run.stdout)
	gotest.Len(t, run.calls, 1, "no --help probe: calls %v", run.calls)
	gotest.NotContains(t, run.calls[0], "--events-file")
	gotest.NotContains(t, run.output, "stream=")
	gotest.NotContains(t, run.output, "profile=")
}

func (s *ActionAuditTestSuite) TestAnAttestedPushKeepsItsStream(t *gotest.T) {
	run := s.runStep(t, t.TempDir(), "test", map[string]string{"attest": "true"}, "EVENT_NAME=push")
	gotest.Equal(t, 0, run.exit, "stdout: %s", run.stdout)
	gotest.Contains(t, run.output, "stream="+filepath.Join(run.temp, "gotest-run-events.json")+"\n")
}

func (s *ActionAuditTestSuite) TestAnUnknownAuditValueFailsBeforeRunning(t *gotest.T) {
	run := s.runStep(t, t.TempDir(), "test", map[string]string{"audit": "yes"})
	gotest.Equal(t, 2, run.exit)
	gotest.Contains(t, run.stdout, "::error::audit must be auto, true or false; got yes")
	gotest.Empty(t, run.calls)
}

func (s *ActionAuditTestSuite) TestTheAuditComparesWithThePullRequestBase(t *gotest.T) {
	dir, base, _ := twoCommits(t)
	stream, profile := filepath.Join(dir, "events.json"), filepath.Join(dir, "cover.out")
	gotest.NoError(t, os.WriteFile(stream, []byte("{}\n"), 0o600))
	gotest.NoError(t, os.WriteFile(profile, []byte("mode: set\n"), 0o600))
	run := s.runStep(t, dir, "audit", map[string]string{"packages": "./a/...\n./b"},
		"EVENT_NAME=pull_request", "PR_BASE="+base, "STREAM="+stream, "PROFILE="+profile, "STANDIN_AUDIT_EXIT=2")
	gotest.Equal(t, 0, run.exit, "without audit-fail-on the step passes: stdout: %s", run.stdout)
	report := filepath.Join(run.temp, "gotest-audit.md")
	gotest.Equal(t, "audit --base="+base+" --format=md --output="+report+" --evidence="+stream+" --coverage="+profile+" ./a/... ./b", run.call("audit --base"))
	gotest.Contains(t, run.summary, "## gotest audit from the stand-in\n")
	gotest.Contains(t, run.output, "audit-report="+report+"\n")
	gotest.Contains(t, run.stdout, "::warning title=gotest audit::gotest audit exited 2")
}

func (s *ActionAuditTestSuite) TestAuditFailOnFailsTheStep(t *gotest.T) {
	dir, base, _ := twoCommits(t)
	run := s.runStep(t, dir, "audit", map[string]string{"audit-fail-on": "removed,focus"},
		"EVENT_NAME=pull_request", "PR_BASE="+base, "STANDIN_AUDIT_EXIT=1")
	gotest.Equal(t, 1, run.exit)
	call := run.call("audit --base")
	gotest.Contains(t, call, " --fail-on=removed,focus ")
	gotest.NotContains(t, call, "--evidence", "no stream was kept")
	gotest.Contains(t, run.summary, "## gotest audit from the stand-in\n", "the report is posted before the step fails")
}

func (s *ActionAuditTestSuite) TestTheAuditMutatesOnlyWhenAsked(t *gotest.T) {
	dir, base, _ := twoCommits(t)
	for t, tc := range gotest.Each(t, []struct {
		Desc, input string
		env         []string
		args        string
		exit        int
		stdout      string
	}{
		{Desc: "not asked", input: "", args: ""},
		{Desc: "asked with a budget", input: "5m", args: " --mutate --mutate-budget=5m"},
		{Desc: "asked with the default budget", input: "true", args: " --mutate"},
		{Desc: "asked of a gotest without it", input: "5m", env: []string{"STANDIN_NO_MUTATE=1"}, stdout: "::warning title=gotest audit::the gotest in use has no audit --mutate; the audit mutates nothing"},
		{Desc: "asked with no duration", input: "soon", exit: 2, stdout: "::error::audit-mutate must be empty, true or a duration such as 5m; got soon"},
	}) {
		run := s.runStep(t, dir, "audit", map[string]string{"audit-mutate": tc.input}, append([]string{"EVENT_NAME=pull_request", "PR_BASE=" + base}, tc.env...)...)
		gotest.Equal(t, tc.exit, run.exit, "stdout: %s", run.stdout)
		if tc.stdout != "" {
			gotest.Contains(t, run.stdout, tc.stdout)
		}
		if tc.exit != 0 {
			gotest.Empty(t, run.call("audit --base"))
			continue
		}
		report := filepath.Join(run.temp, "gotest-audit.md")
		gotest.Equal(t, "audit --base="+base+" --format=md --output="+report+tc.args+" ./...", run.call("audit --base"))
	}
}

func (s *ActionAuditTestSuite) TestAShallowCheckoutFetchesTheBase(t *gotest.T) {
	origin, base, head := twoCommits(t)
	parent, err := filepath.EvalSymlinks(t.TempDir())
	gotest.NoError(t, err)
	dir := filepath.Join(parent, "checkout")
	auditGit(t, parent, "clone", "-q", "--depth=1", "file://"+origin, dir)
	gotest.Equal(t, head, auditGit(t, dir, "rev-parse", "HEAD"))
	cmd := exec.CommandContext(t.Context(), "git", "-C", dir, "cat-file", "-e", base+"^{commit}") //nolint:gosec // G204: git on the test's own clone
	gotest.Error(t, cmd.Run(), "the clone lacks the base")

	run := s.runStep(t, dir, "audit", nil, "EVENT_NAME=pull_request", "PR_BASE="+base)
	gotest.Equal(t, 0, run.exit, "stdout: %s\nstderr: %s", run.stdout, run.stderr)
	gotest.Contains(t, run.call("audit --base"), "audit --base="+base+" ")
	auditGit(t, dir, "cat-file", "-e", base+"^{commit}")
}

func (s *ActionAuditTestSuite) TestAnUnfetchableBaseSkipsTheAudit(t *gotest.T) {
	dir, _, _ := twoCommits(t)
	missing := strings.Repeat("ab", 20)
	run := s.runStep(t, dir, "audit", nil, "EVENT_NAME=pull_request", "PR_BASE="+missing)
	gotest.Equal(t, 0, run.exit, "stdout: %s", run.stdout)
	gotest.Empty(t, run.call("audit --base"))
	gotest.Contains(t, run.summary, "> gotest audit skipped: the base commit "+missing+" could not be fetched\n")
}

func (s *ActionAuditTestSuite) TestWhenTheAuditDoesNotRun(t *gotest.T) {
	dir, base, _ := twoCommits(t)
	t.When("audit is auto on a push", func(w *gotest.T) {
		run := s.runStep(w, dir, "audit", nil, "EVENT_NAME=push", "PUSH_BEFORE="+base)
		w.It("does nothing", func(it *gotest.T) {
			gotest.Equal(it, 0, run.exit)
			gotest.Empty(it, run.calls)
			gotest.Empty(it, run.summary)
		})
	})
	t.When("audit is true on a push", func(w *gotest.T) {
		run := s.runStep(w, dir, "audit", map[string]string{"audit": "true"}, "EVENT_NAME=push", "PUSH_BEFORE="+base)
		w.It("compares with the commit before the push", func(it *gotest.T) {
			gotest.Equal(it, 0, run.exit, "stdout: %s", run.stdout)
			gotest.Contains(it, run.call("audit --base"), "audit --base="+base+" ")
		})
	})
	t.When("audit is true on a push that created the branch", func(w *gotest.T) {
		run := s.runStep(w, dir, "audit", map[string]string{"audit": "true"}, "EVENT_NAME=push", "PUSH_BEFORE="+strings.Repeat("0", 40))
		w.It("says the event names no base", func(it *gotest.T) {
			gotest.Equal(it, 0, run.exit)
			gotest.Empty(it, run.call("audit --base"))
			gotest.Contains(it, run.summary, "> gotest audit skipped: the push event names no base commit\n")
		})
	})
}

func (s *ActionAuditTestSuite) TestThePredicateNamesTheTestedCommit(t *gotest.T) {
	dir, _, head := twoCommits(t)
	stream := filepath.Join(dir, "events.json")
	gotest.NoError(t, os.WriteFile(stream, []byte("{}\n"), 0o600))
	run := s.runStep(t, dir, "predicate", nil, "STREAM="+stream, "GITHUB_SERVER_URL=https://github.example", "GITHUB_REPOSITORY=o/r")
	gotest.Equal(t, 0, run.exit, "stdout: %s\nstderr: %s", run.stdout, run.stderr)
	predicate := filepath.Join(run.temp, "gotest-test-result.json")
	gotest.Equal(t, "summary --input="+stream+" --format=intoto-test-result --output="+predicate, run.call("summary"))

	archive := exec.CommandContext(t.Context(), "git", "-C", dir, "archive", "--format=tar", head) //nolint:gosec // G204: git on the test's own repository
	tar, err := archive.Output()
	gotest.NoError(t, err)
	sum := sha256.Sum256(tar)
	gotest.Contains(t, run.output, "path="+predicate+"\n")
	gotest.Contains(t, run.output, "subject-name=git+https://github.example/o/r@"+head+"\n")
	gotest.Contains(t, run.output, "subject-digest=sha256:"+hex.EncodeToString(sum[:])+"\n")
}

func (s *ActionAuditTestSuite) TestAPredicateWithoutAStreamIsSkipped(t *gotest.T) {
	dir, _, _ := twoCommits(t)
	run := s.runStep(t, dir, "predicate", nil)
	gotest.Equal(t, 0, run.exit)
	gotest.Empty(t, run.calls)
	gotest.NotContains(t, run.output, "path=")
	gotest.Contains(t, run.summary, "> gotest attestation skipped: the run saved no event stream\n")
}

func (s *ActionAuditTestSuite) TestTheAttestationIsSoftAndTyped(t *gotest.T) {
	action := readAction(t)
	attest := actionStepNamed(t, action, "attest")
	gotest.Regexp(t, `^actions/attest@[0-9a-f]{40}$`, attest.Uses)
	gotest.Equal(t, "https://in-toto.io/attestation/test-result/v0.1", attest.With["predicate-type"])
	gotest.Equal(t, "${{ steps.predicate.outputs.path }}", attest.With["predicate-path"])
	gotest.Equal(t, "${{ steps.predicate.outputs.subject-digest }}", attest.With["subject-digest"])
	gotest.Equal(t, "true", attest.ContinueOnErr.Value, "an attestation the token cannot make does not fail the job")
	report := actionStepNamed(t, action, "Report a failed attestation")
	gotest.Contains(t, report.If, "steps.attest.outcome == 'failure'")
	run := runActionStep(t, report.Run, t.TempDir(), nil)
	gotest.Equal(t, 0, run.exit)
	gotest.Contains(t, run.summary, "id-token: write and attestations: write")
}
