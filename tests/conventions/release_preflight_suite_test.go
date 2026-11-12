package conventions_test

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// ReleasePreflightTestSuite runs the release preflight in a scratch
// repository with a local origin and stand-ins for curl and gh: a version
// below a floor the commit ships is refused before the network is asked.
type ReleasePreflightTestSuite struct{}

func (s *ReleasePreflightTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second}
}

func (s *ReleasePreflightTestSuite) SuiteGuard() string {
	if runtime.GOOS == "windows" {
		return "the stand-ins are shell scripts"
	}
	if _, err := exec.LookPath("bash"); err != nil {
		return "bash not found"
	}
	if _, err := exec.LookPath("git"); err != nil {
		return "git not found"
	}
	return ""
}

// preflightStandIn logs its call and answers as the proxy's empty list
// (curl) or as a green run (gh).
const preflightStandIn = `#!/usr/bin/env bash
echo "$(basename "$0") $*" >> "$STANDIN_LOG"
[ "$(basename "$0")" = gh ] && echo "completed success https://example.invalid/run"
exit 0
`

// isolatedGit keeps the user's git configuration out of the scratch repository.
var isolatedGit = []string{"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1"}

type preflightRepo struct{ dir, bin, sha, log string }

// stagePreflight makes a repository on main with go.mod and the two floor
// files, pushed to a bare origin with tag v1.37.0, and a bin directory
// holding the stand-ins. An empty floor leaves the constant out of its file.
func stagePreflight(t *gotest.T, runtimeFloor, cliFloor string) preflightRepo {
	r := preflightRepo{dir: t.TempDir(), bin: t.TempDir()}
	r.log = filepath.Join(r.bin, "calls")
	origin := filepath.Join(t.TempDir(), "origin.git")
	runtimeSrc, cliSrc := "package gotestgen\n", ""
	if runtimeFloor != "" {
		runtimeSrc += "\nconst MinRuntimeVersion = \"" + runtimeFloor + "\"\n"
	}
	if cliFloor != "" {
		cliSrc = "export const MIN_CLI_VERSION = \"" + cliFloor + "\";\n"
	}
	r.write(t, "go.mod", "module example.com/rel\n")
	r.write(t, "internal/gotestgen/runtimeversion.go", runtimeSrc)
	r.write(t, "vscode-gotest/src/cli.ts", cliSrc)
	for _, name := range []string{"curl", "gh"} {
		gotest.NoError(t, os.WriteFile(filepath.Join(r.bin, name), []byte(preflightStandIn), 0o700)) //nolint:gosec // G306: an executable
	}
	r.git(t, "init", "-q", "-b", "main")
	r.git(t, "add", "-A")
	r.commit(t, "base")
	r.git(t, "tag", "v1.37.0")
	r.git(t, "init", "-q", "--bare", "-b", "main", origin)
	r.git(t, "remote", "add", "origin", origin)
	r.git(t, "push", "-q", "origin", "main", "v1.37.0")
	r.sha = strings.TrimSpace(r.git(t, "rev-parse", "HEAD"))
	return r
}

func (r preflightRepo) write(t *gotest.T, rel, content string) {
	path := filepath.Join(r.dir, filepath.FromSlash(rel))
	gotest.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	gotest.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

func (r preflightRepo) commit(t *gotest.T, msg string, args ...string) {
	r.git(t, append([]string{"-c", "user.name=t", "-c", "user.email=t@example.invalid", "-c", "commit.gpgsign=false", "commit", "-q", "-m", msg}, args...)...)
}

func (r preflightRepo) git(t *gotest.T, args ...string) string {
	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = r.dir
	cmd.Env = append(os.Environ(), isolatedGit...)
	out, err := cmd.CombinedOutput()
	gotest.NoError(t, err, "git %s: %s", strings.Join(args, " "), out)
	return string(out)
}

// run runs the preflight in the repository and returns its exit code, stdout and stderr.
func (r preflightRepo) run(t *gotest.T, args ...string) (exit int, stdout, stderr string) {
	script, err := filepath.Abs(filepath.Join("..", "..", ".github", "scripts", "release-preflight.sh"))
	gotest.NoError(t, err)
	cmd := exec.CommandContext(t.Context(), "bash", append([]string{script}, args...)...) //nolint:gosec // G204: the repository's own script
	cmd.Dir = r.dir
	var out, errOut strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errOut
	cmd.Env = append(os.Environ(), isolatedGit...)
	cmd.Env = append(cmd.Env, "PWD="+r.dir,
		"PATH="+r.bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"STANDIN_LOG="+r.log, "GITHUB_REPOSITORY=example/rel")
	_ = cmd.Run()
	return cmd.ProcessState.ExitCode(), out.String(), errOut.String()
}

func (s *ReleasePreflightTestSuite) TestBelowTheRuntimeFloorIsRefused(t *gotest.T) {
	r := stagePreflight(t, "v1.38.0", "v1.38.0")
	exit, stdout, stderr := r.run(t, "v1.37.1")
	gotest.Equal(t, 1, exit, "stdout: %s\nstderr: %s", stdout, stderr)
	gotest.Contains(t, stderr, "preflight: v1.37.1 is below MinRuntimeVersion v1.38.0 at ", "stdout: %s\nstderr: %s", stdout, stderr)
	gotest.Contains(t, stderr, "release v1.38.0 or later", "stdout: %s\nstderr: %s", stdout, stderr)
	_, err := os.Stat(r.log)
	gotest.ErrorIs(t, err, fs.ErrNotExist, "nothing may ask the proxy or gh; stdout: %s\nstderr: %s", stdout, stderr)
}

func (s *ReleasePreflightTestSuite) TestBelowTheExtensionFloorIsRefused(t *gotest.T) {
	r := stagePreflight(t, "v1.37.0", "v1.38.0")
	exit, stdout, stderr := r.run(t, "v1.37.1")
	gotest.Equal(t, 1, exit, "stdout: %s\nstderr: %s", stdout, stderr)
	gotest.Contains(t, stderr, "preflight: v1.37.1 is below MIN_CLI_VERSION v1.38.0 at ", "stdout: %s\nstderr: %s", stdout, stderr)
	gotest.Contains(t, stdout, "ok    v1.37.1 is not below MinRuntimeVersion v1.37.0", "stdout: %s\nstderr: %s", stdout, stderr)
}

func (s *ReleasePreflightTestSuite) TestTheFloorItselfMayBeReleased(t *gotest.T) {
	r := stagePreflight(t, "v1.38.0", "v1.38.0")
	exit, stdout, stderr := r.run(t, "v1.38.0")
	gotest.Equal(t, 0, exit, "stdout: %s\nstderr: %s", stdout, stderr)
	gotest.Contains(t, stdout, "ok    v1.38.0 is not below MinRuntimeVersion v1.38.0", "stdout: %s\nstderr: %s", stdout, stderr)
	gotest.Contains(t, stdout, "ok    v1.38.0 is not below MIN_CLI_VERSION v1.38.0", "stdout: %s\nstderr: %s", stdout, stderr)
	gotest.Contains(t, stdout, "preflight: v1.38.0 may be released from "+r.sha, "stdout: %s\nstderr: %s", stdout, stderr)
}

func (s *ReleasePreflightTestSuite) TestAboveTheFloorsMayBeReleased(t *gotest.T) {
	r := stagePreflight(t, "v1.38.0", "v1.38.0")
	exit, stdout, stderr := r.run(t, "v1.39.0")
	gotest.Equal(t, 0, exit, "stdout: %s\nstderr: %s", stdout, stderr)
	gotest.Contains(t, stdout, "preflight: v1.39.0 may be released from "+r.sha, "stdout: %s\nstderr: %s", stdout, stderr)
}

func (s *ReleasePreflightTestSuite) TestTheFloorsAreReadFromTheCommit(t *gotest.T) {
	r := stagePreflight(t, "v1.38.0", "v1.38.0")
	r.write(t, "internal/gotestgen/runtimeversion.go", "package gotestgen\n\nconst MinRuntimeVersion = \"v1.0.0\"\n")
	r.write(t, "vscode-gotest/src/cli.ts", "export const MIN_CLI_VERSION = \"v1.0.0\";\n")
	exit, stdout, stderr := r.run(t, "v1.37.1")
	gotest.Equal(t, 1, exit, "stdout: %s\nstderr: %s", stdout, stderr)
	gotest.Contains(t, stderr, "is below MinRuntimeVersion v1.38.0", "stdout: %s\nstderr: %s", stdout, stderr)
}

func (s *ReleasePreflightTestSuite) TestTheReleaseWorkflowIsRefusedToo(t *gotest.T) {
	r := stagePreflight(t, "v1.38.0", "v1.38.0")
	r.git(t, "tag", "v1.37.1")
	r.git(t, "push", "-q", "origin", "v1.37.1")
	exit, stdout, stderr := r.run(t, "--tagged", "v1.37.1", r.sha)
	gotest.Equal(t, 1, exit, "stdout: %s\nstderr: %s", stdout, stderr)
	gotest.Contains(t, stderr, "preflight: v1.37.1 is below MinRuntimeVersion v1.38.0 at ", "stdout: %s\nstderr: %s", stdout, stderr)
}

func (s *ReleasePreflightTestSuite) TestAFloorThatCannotBeReadIsRefused(t *gotest.T) {
	r := stagePreflight(t, "v1.38.0", "")
	exit, stdout, stderr := r.run(t, "v1.38.0")
	gotest.Equal(t, 1, exit, "stdout: %s\nstderr: %s", stdout, stderr)
	gotest.Contains(t, stderr, "preflight: cannot read MIN_CLI_VERSION from vscode-gotest/src/cli.ts at ", "stdout: %s\nstderr: %s", stdout, stderr)
}

func (s *ReleasePreflightTestSuite) TestTheRepositorysOwnFloorFilesAreRead(t *gotest.T) {
	r := stagePreflight(t, "v1.38.0", "v1.38.0")
	for _, rel := range []string{"internal/gotestgen/runtimeversion.go", "vscode-gotest/src/cli.ts"} {
		data, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(rel)))
		gotest.NoError(t, err)
		r.write(t, rel, string(data))
	}
	r.commit(t, "real", "-a")
	r.git(t, "push", "-q", "origin", "main")
	exit, stdout, stderr := r.run(t, "v999.0.0")
	gotest.Equal(t, 0, exit, "stdout: %s\nstderr: %s", stdout, stderr)
	gotest.Regexp(t, `(?m)^ok    v999\.0\.0 is not below MinRuntimeVersion `, stdout, "stdout: %s\nstderr: %s", stdout, stderr)
	gotest.Regexp(t, `(?m)^ok    v999\.0\.0 is not below MIN_CLI_VERSION `, stdout, "stdout: %s\nstderr: %s", stdout, stderr)
}
