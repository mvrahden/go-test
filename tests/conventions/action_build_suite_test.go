package conventions_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// ActionBuildTestSuite runs the resolve step with a go that records its
// builds: every decision builds the CLI once, and the steps run that binary.
type ActionBuildTestSuite struct{}

func (s *ActionBuildTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second}
}

func (s *ActionBuildTestSuite) SuiteGuard() string { return unixActionGuard() }

// goWrapper passes every go command through except build, which it records
// and answers with a stand-in binary at the -o path.
const goWrapper = `#!/usr/bin/env bash
if [ "$1" != build ]; then exec "$REAL_GO" "$@"; fi
echo "$*" >> "$GO_LOG"
[ -n "$GO_BUILD_FAILS" ] && { echo "go: no required module provides package" >&2; exit 1; }
prev=""; for a in "$@"; do [ "$prev" = -o ] && out="$a"; prev="$a"; done
printf '#!/usr/bin/env bash\nexit 0\n' > "$out" && chmod +x "$out"
`

// exitingCLI exits with STANDIN_EXIT.
const exitingCLI = "#!/usr/bin/env bash\nexit \"${STANDIN_EXIT:-0}\"\n"

const (
	consumerMod  = "module example.com/consumer\n\ngo 1.26.0\n\nrequire github.com/mvrahden/go-test v1.37.0\n"
	consumerTool = "module example.com/consumer\n\ngo 1.26.0\n\ntool " + gotestPkg + "\n\nrequire github.com/mvrahden/go-test v1.37.0\n"
	consumerSum  = "example.com/kept v1.0.0 h1:kept=\n"
)

// consumerShapes are the three decisions: a declared tool, a workspace, neither.
var consumerShapes = map[string]map[string]string{
	"declared":  {"go.mod": consumerTool},
	"workspace": {"go.mod": consumerMod, "go.work": "go 1.26.0\n\nuse .\n"},
	"plain":     {"go.mod": consumerMod, "go.sum": consumerSum},
}

// buildRun is a resolve run and the go build lines it logged.
type buildRun struct {
	actionRun
	builds []string
}

// resolve runs the resolve step in a fresh copy of shape.
func (s *ActionBuildTestSuite) resolve(t *gotest.T, shape string, env ...string) buildRun {
	realGo, err := exec.LookPath("go")
	gotest.NoError(t, err)
	dir := t.TempDir()
	for name, content := range consumerShapes[shape] {
		gotest.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
	}
	log := filepath.Join(t.TempDir(), "go.log")
	run := runActionStep(t, actionStep(t, "resolve"), dir, map[string]string{"go": goWrapper},
		append([]string{"REAL_GO=" + realGo, "GO_LOG=" + log,
			"GOWORK=", "GOFLAGS=", "GOTOOLCHAIN=local", "GOPROXY=off"}, env...)...)
	data, _ := os.ReadFile(log)
	builds := []string{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line != "" {
			builds = append(builds, line)
		}
	}
	return buildRun{run, builds}
}

func (s *ActionBuildTestSuite) TestADeclaredToolIsBuiltOnce(t *gotest.T) {
	run := s.resolve(t, "declared")
	exe := filepath.Join(run.temp, "gotest-bin", "gotest")
	gotest.Equal(t, 0, run.exit, "stdout: %s\nstderr: %s", run.stdout, run.stderr)
	gotest.Equal(t, []string{"build -o " + exe + " " + gotestPkg}, run.builds)
	gotest.Contains(t, run.env, "GOTEST_BIN="+exe+"\n")
	gotest.NotContains(t, run.env, "GOTEST_CMD=")
}

func (s *ActionBuildTestSuite) TestAWorkspaceBuildsWithoutModFlags(t *gotest.T) {
	run := s.resolve(t, "workspace")
	gotest.Equal(t, 0, run.exit, "stdout: %s\nstderr: %s", run.stdout, run.stderr)
	gotest.Equal(t, []string{"build -o " + filepath.Join(run.temp, "gotest-bin", "gotest") + " " + gotestPkg}, run.builds)
}

func (s *ActionBuildTestSuite) TestAnUndeclaredToolIsBuiltBesideTheCheckout(t *gotest.T) {
	run := s.resolve(t, "plain")
	bin := filepath.Join(run.temp, "gotest-bin")
	gotest.Equal(t, 0, run.exit, "stdout: %s\nstderr: %s", run.stdout, run.stderr)
	gotest.Equal(t, []string{"build -mod=mod -modfile=" + filepath.Join(bin, "go.mod") + " -o " + filepath.Join(bin, "gotest") + " " + gotestPkg}, run.builds)
	sum, err := os.ReadFile(filepath.Join(run.dir, "go.sum"))
	gotest.NoError(t, err)
	gotest.Equal(t, consumerSum, string(sum))
	copied, err := os.ReadFile(filepath.Join(bin, "go.sum"))
	gotest.NoError(t, err)
	gotest.Equal(t, string(sum), string(copied))
}

func (s *ActionBuildTestSuite) TestABuildThatFailsNamesTheFix(t *gotest.T) {
	run := s.resolve(t, "declared", "GO_BUILD_FAILS=1")
	gotest.Equal(t, 2, run.exit)
	gotest.Contains(t, run.stdout, "::error title=gotest::could not build "+gotestPkg+" as go.mod selects it; run go mod tidy, or declare the tool with: go get -tool "+gotestPkg+"@<pinned version>")
	gotest.NotContains(t, run.env, "GOTEST_BIN=")
}

func (s *ActionBuildTestSuite) TestTheExitCodeOutputIsTheCLIsOwn(t *gotest.T) {
	for sub, code := range gotest.Each(t, []int{0, 1, 2, 3, 130}) {
		cli := filepath.Join(sub.TempDir(), "gotest")
		gotest.NoError(sub, os.WriteFile(cli, []byte(exitingCLI), 0o700)) //nolint:gosec // G306: an executable
		run := runActionStep(sub, actionStep(sub, "test"), sub.TempDir(), nil,
			"INPUT_VERSION=gomod", "INPUT_PACKAGES=./...", "GOTEST_BIN="+cli, "STANDIN_EXIT="+strconv.Itoa(code))
		gotest.Equal(sub, code, run.exit, "stdout: %s", run.stdout)
		gotest.Contains(sub, run.output, "exit-code="+strconv.Itoa(code)+"\n")
	}
}

func (s *ActionBuildTestSuite) TestNoStepRunsTheCLIThroughGoRun(t *gotest.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "action.yml"))
	gotest.NoError(t, err)
	gotest.NotContains(t, string(data), "go run ")
	gotest.NotContains(t, string(data), "GOTEST_CMD")
	gotest.Contains(t, string(data), "steps.test.outputs.exit-code != '2'")
}
