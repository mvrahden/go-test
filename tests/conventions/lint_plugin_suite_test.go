package conventions_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// LintPluginTestSuite runs make lint-plugin on staged modules: the nested
// plugin module is vetted and tested on its own, outside any workspace.
type LintPluginTestSuite struct{}

func (s *LintPluginTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: time.Minute}
}

func (s *LintPluginTestSuite) SuiteGuard() string {
	if runtime.GOOS == "windows" {
		return "the target's recipe is a POSIX shell line"
	}
	if _, err := exec.LookPath("make"); err != nil {
		return "make not found"
	}
	return ""
}

// lintPlugin runs the target on dir and returns its output and exit code.
func (s *LintPluginTestSuite) lintPlugin(t *gotest.T, dir string) (string, int) {
	cmd := exec.CommandContext(t.Context(), "make", "-s", "lint-plugin", "PLUGIN_DIR="+dir)
	cmd.Dir = filepath.Join("..", "..")
	cmd.Env = append(os.Environ(), "GOFLAGS=")
	out, _ := cmd.CombinedOutput()
	return string(out), cmd.ProcessState.ExitCode()
}

// stagePlugin writes a module whose test passes or fails, below a go.work
// that does not list it: only GOWORK=off builds it.
func (s *LintPluginTestSuite) stagePlugin(t *gotest.T, pass bool) string {
	root := t.TempDir()
	dir := filepath.Join(root, "plugin")
	want := "1"
	if !pass {
		want = "2"
	}
	files := map[string]string{
		filepath.Join(root, "go.work"):     "go 1.26.0\n",
		filepath.Join(dir, "go.mod"):       "module plugmod\n\ngo 1.26.0\n",
		filepath.Join(dir, "plug.go"):      "package plug\n\nfunc One() int { return 1 }\n",
		filepath.Join(dir, "plug_test.go"): "package plug\n\nimport \"testing\"\n\nfunc TestOne(t *testing.T) {\n\tif One() != " + want + " {\n\t\tt.Fatal(\"One\")\n\t}\n}\n",
	}
	for path, content := range files {
		gotest.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
		gotest.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}
	return dir
}

func (s *LintPluginTestSuite) TestAnAbsentPluginIsNothingToCheck(t *gotest.T) {
	missing := filepath.Join(t.TempDir(), "golangci")
	out, code := s.lintPlugin(t, missing)
	gotest.Equal(t, 0, code, "output: %s", out)
	gotest.Equal(t, "lint-plugin: no module at "+missing+", nothing to check\n", out)
}

func (s *LintPluginTestSuite) TestAPluginIsVettedAndTestedOutsideTheWorkspace(t *gotest.T) {
	out, code := s.lintPlugin(t, s.stagePlugin(t, true))
	gotest.Equal(t, 0, code, "output: %s", out)
	gotest.Contains(t, out, "ok  \tplugmod")
}

func (s *LintPluginTestSuite) TestAFailingPluginTestFailsTheTarget(t *gotest.T) {
	out, code := s.lintPlugin(t, s.stagePlugin(t, false))
	gotest.NotEqual(t, 0, code, "output: %s", out)
	gotest.Contains(t, out, "FAIL\tplugmod")
}

func (s *LintPluginTestSuite) TestTheGateRunsIt(t *gotest.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "Makefile"))
	gotest.NoError(t, err)
	gotest.Regexp(t, `(?m)^checks:.* lint-plugin\b`, string(data))
}
