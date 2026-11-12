package conventions_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// ActionResolveTestSuite runs the action's resolve step with a stand-in go
// that fakes only the tool listing: the tool directive in go.mod decides,
// whichever format the Go release lists it in.
type ActionResolveTestSuite struct {
	script, realGo string
}

func (s *ActionResolveTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second}
}

func (s *ActionResolveTestSuite) SuiteGuard() string {
	if runtime.GOOS == "windows" {
		return "the stand-in go is a shell script"
	}
	if _, err := exec.LookPath("bash"); err != nil {
		return "bash not found"
	}
	return ""
}

func (s *ActionResolveTestSuite) BeforeAll(t *gotest.T) {
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
		if step.ID == "resolve" {
			s.script = step.Run
		}
	}
	gotest.NotEmpty(t, s.script, "action.yml has no step with id resolve")
	s.realGo, err = exec.LookPath("go")
	gotest.NoError(t, err)
}

// AfterAll has nothing to release: every run lives in a temp dir.
func (s *ActionResolveTestSuite) AfterAll(t *gotest.T) {}

// goStandIn prints the faked listing for a bare go tool and hands every
// other call to the real toolchain.
const goStandIn = `#!/usr/bin/env bash
if [ "$1" = tool ] && [ $# -eq 1 ]; then
  printf '%s\n' "$FAKE_TOOL_LISTING"
  exit 0
fi
exec "$REAL_GO" "$@"
`

const gotestPkg = "github.com/mvrahden/go-test/cmd/gotest"

const (
	listingGo126 = "asm\ncgo\ncompile\nvet\n" + gotestPkg               // Go 1.26: the bare path
	listingGo127 = "asm\ncgo\ncompile\nvet\ngotest (" + gotestPkg + ")" // Go 1.27: name (path)
	listingNone  = "asm\ncgo\ncompile\nvet"
)

const (
	modTool      = "module example.com/app\n\ngo 1.26.0\n\ntool " + gotestPkg + "\n\nrequire github.com/mvrahden/go-test v1.37.0\n"
	modToolBlock = "module example.com/app\n\ngo 1.26.0\n\ntool (\n\tgolang.org/x/tools/cmd/stringer\n\t" + gotestPkg + "\n)\n\nrequire github.com/mvrahden/go-test v1.37.0\n"
	modRequire   = "module example.com/app\n\ngo 1.26.0\n\ntool golang.org/x/tools/cmd/stringer\n\nrequire github.com/mvrahden/go-test v1.37.0\n"
	modPlain     = "module example.com/lib\n\ngo 1.26.0\n"
	modGotest    = "module github.com/mvrahden/go-test\n\ngo 1.26.0\n"
	workAB       = "go 1.26.0\n\nuse (\n\t./a\n\t./b\n)\n"
)

const (
	wantTool      = "GOTEST_CMD=go tool " + gotestPkg + "\n"
	wantRun       = "GOTEST_CMD=go run " + gotestPkg + "\n"
	wantRunMod    = "GOTEST_CMD=go run -mod=mod " + gotestPkg + "\n"
	resolveNotice = "::notice title=gotest::go.mod does not declare the gotest tool"
)

type resolveRun struct {
	exit        int
	stdout, env string // env is what the step appended to GITHUB_ENV
}

// resolve writes files (path relative to a fresh directory -> content), runs
// the resolve step in cwd under it with the stand-in go printing listing, and
// returns what the step did.
func (s *ActionResolveTestSuite) resolve(t *gotest.T, files map[string]string, cwd, listing string) resolveRun {
	bin, root := t.TempDir(), t.TempDir()
	gotest.NoError(t, os.WriteFile(filepath.Join(bin, "go"), []byte(goStandIn), 0o700)) //nolint:gosec // G306: an executable
	for rel, content := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		gotest.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
		gotest.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}
	dir := filepath.Join(root, filepath.FromSlash(cwd))
	env := filepath.Join(bin, "env")
	cmd := exec.CommandContext(t.Context(), "bash", "--noprofile", "--norc", "-eo", "pipefail", "-c", s.script) //nolint:gosec // G204: the action's own script
	cmd.Dir = dir
	// GOPROXY=off: the step must decide from the files alone.
	cmd.Env = append(os.Environ(), "PWD="+dir,
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"GITHUB_ENV="+env, "REAL_GO="+s.realGo, "FAKE_TOOL_LISTING="+listing,
		"GOWORK=", "GOFLAGS=", "GOTOOLCHAIN=local", "GOPROXY=off")
	out, _ := cmd.Output()
	written, _ := os.ReadFile(env)
	return resolveRun{exit: cmd.ProcessState.ExitCode(), stdout: string(out), env: string(written)}
}

func (s *ActionResolveTestSuite) TestADeclaredToolIsFoundUnderGo126(t *gotest.T) {
	run := s.resolve(t, map[string]string{"go.mod": modTool}, ".", listingGo126)
	gotest.Equal(t, 0, run.exit, "stdout: %s", run.stdout)
	gotest.Equal(t, wantTool, run.env, "stdout: %s", run.stdout)
	gotest.NotContains(t, run.stdout, resolveNotice)
}

func (s *ActionResolveTestSuite) TestADeclaredToolIsFoundUnderGo127(t *gotest.T) {
	run := s.resolve(t, map[string]string{"go.mod": modTool}, ".", listingGo127)
	gotest.Equal(t, 0, run.exit, "stdout: %s", run.stdout)
	gotest.Equal(t, wantTool, run.env, "stdout: %s", run.stdout)
	gotest.NotContains(t, run.stdout, resolveNotice)
}

func (s *ActionResolveTestSuite) TestAToolInABlockIsFound(t *gotest.T) {
	run := s.resolve(t, map[string]string{"go.mod": modToolBlock}, ".", listingGo126)
	gotest.Equal(t, 0, run.exit, "stdout: %s", run.stdout)
	gotest.Equal(t, wantTool, run.env, "stdout: %s", run.stdout)
	gotest.NotContains(t, run.stdout, resolveNotice)
}

func (s *ActionResolveTestSuite) TestAToolOfAnyWorkspaceModuleIsFound(t *gotest.T) {
	run := s.resolve(t, map[string]string{"go.work": workAB, "a/go.mod": modPlain, "b/go.mod": modTool}, ".", listingGo126)
	gotest.Equal(t, 0, run.exit, "stdout: %s", run.stdout)
	gotest.Equal(t, wantTool, run.env, "stdout: %s", run.stdout)
	gotest.NotContains(t, run.stdout, resolveNotice)
}

func (s *ActionResolveTestSuite) TestARequiredModuleIsNotATool(t *gotest.T) {
	run := s.resolve(t, map[string]string{"go.mod": modRequire}, ".", listingNone)
	gotest.Equal(t, 0, run.exit, "stdout: %s", run.stdout)
	gotest.Equal(t, wantRunMod, run.env, "stdout: %s", run.stdout)
	gotest.Contains(t, run.stdout, resolveNotice)
}

func (s *ActionResolveTestSuite) TestTheListingAloneDeclaresNothing(t *gotest.T) {
	run := s.resolve(t, map[string]string{"go.mod": modRequire}, ".", listingGo127)
	gotest.Equal(t, 0, run.exit, "stdout: %s", run.stdout)
	gotest.Equal(t, wantRunMod, run.env, "stdout: %s", run.stdout)
	gotest.Contains(t, run.stdout, resolveNotice)
}

func (s *ActionResolveTestSuite) TestAWorkspaceWithoutTheToolRunsFromSource(t *gotest.T) {
	run := s.resolve(t, map[string]string{"go.work": workAB, "a/go.mod": modPlain, "b/go.mod": modRequire}, ".", listingNone)
	gotest.Equal(t, 0, run.exit, "stdout: %s", run.stdout)
	gotest.Equal(t, wantRun, run.env, "stdout: %s", run.stdout)
	gotest.Contains(t, run.stdout, resolveNotice)
}

func (s *ActionResolveTestSuite) TestTheGotestWorkspaceIsNotToldToDeclareIt(t *gotest.T) {
	run := s.resolve(t, map[string]string{"go.work": workAB, "a/go.mod": modGotest, "b/go.mod": modPlain}, ".", listingNone)
	gotest.Equal(t, 0, run.exit, "stdout: %s", run.stdout)
	gotest.Equal(t, wantRun, run.env, "stdout: %s", run.stdout)
	gotest.NotContains(t, run.stdout, resolveNotice)
}
