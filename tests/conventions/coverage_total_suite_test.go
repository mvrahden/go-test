package conventions_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// CoverageTotalTestSuite runs coverage-total.sh on a staged module whose
// binary covers what its tests do not.
type CoverageTotalTestSuite struct {
	dir string // read-only after BeforeAll
}

func (s *CoverageTotalTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 2 * time.Minute, SetupTimeout: 2 * time.Minute}
}

// A missing bash skips the suite, except on a Linux runner, where it fails.
func (s *CoverageTotalTestSuite) SuiteGuard() string {
	if runtime.GOOS == "windows" {
		return "the coverage script runs on Linux"
	}
	if _, err := exec.LookPath("bash"); err != nil && (runtime.GOOS != "linux" || os.Getenv("GITHUB_ACTIONS") == "") {
		return "bash not found"
	}
	return ""
}

// goIn runs go in dir with an environment of its own.
func goIn(t *gotest.T, dir string, env []string, args ...string) {
	cmd := exec.CommandContext(t.Context(), "go", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), append([]string{"GOFLAGS=", "GOWORK=off", "GOCOVERDIR="}, env...)...)
	out, err := cmd.CombinedOutput()
	gotest.NoError(t, err, "go %v: %s", args, out)
}

// BeforeAll stages covmod: its test covers A, its binary B.
func (s *CoverageTotalTestSuite) BeforeAll(t *gotest.T) {
	s.dir = t.TempDir()
	files := map[string]string{
		"go.mod":           "module covmod\n\ngo 1.26.0\n",
		"lib.go":           "package lib\n\nfunc A() int { return 1 }\n\nfunc B() int { return 2 }\n",
		"lib_test.go":      "package lib\n\nimport \"testing\"\n\nfunc TestA(t *testing.T) {\n\tif A() != 1 {\n\t\tt.Fatal(\"A\")\n\t}\n}\n",
		"cmd/tool/main.go": "package main\n\nimport lib \"covmod\"\n\nfunc main() { println(lib.B()) }\n",
	}
	for rel, content := range files {
		path := filepath.Join(s.dir, filepath.FromSlash(rel))
		gotest.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
		gotest.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}
	goIn(t, s.dir, nil, "test", "-covermode=atomic", "-coverprofile=suites.out", "./...")
	goIn(t, s.dir, nil, "build", "-cover", "-covermode=atomic", "-o", "tool", "./cmd/tool")
	data := filepath.Join(s.dir, "data")
	gotest.NoError(t, os.MkdirAll(data, 0o750))
	cmd := exec.CommandContext(t.Context(), filepath.Join(s.dir, "tool")) //nolint:gosec // G204: the binary built just above
	cmd.Env = append(os.Environ(), "GOCOVERDIR="+data)
	out, err := cmd.CombinedOutput()
	gotest.NoError(t, err, "tool: %s", out)
	profile, err := os.ReadFile(filepath.Join(s.dir, "suites.out"))
	gotest.NoError(t, err)
	_, rest, _ := strings.Cut(string(profile), "\n")
	gotest.NoError(t, os.WriteFile(filepath.Join(s.dir, "set.out"), []byte("mode: set\n"+rest), 0o600))
}

// AfterAll has nothing to release: the module lives in a temp dir.
func (s *CoverageTotalTestSuite) AfterAll(t *gotest.T) {}

// total runs the script in the staged module, as CI runs it in the
// checkout: go tool cover reads the sources; relative paths are the module's.
func (s *CoverageTotalTestSuite) total(t *gotest.T, profile, data, minimum string, patterns ...string) (string, int) {
	script, err := filepath.Abs(filepath.Join("..", "..", ".github", "scripts", "coverage-total.sh"))
	gotest.NoError(t, err)
	args := []string{script, s.path(profile), s.path(data), minimum}
	cmd := exec.CommandContext(t.Context(), "bash", append(args, patterns...)...) //nolint:gosec // G204: the repository's own script
	cmd.Dir = s.dir
	cmd.Env = append(os.Environ(), "GITHUB_STEP_SUMMARY=", "GOFLAGS=", "GOWORK=off")
	out, _ := cmd.CombinedOutput()
	return string(out), cmd.ProcessState.ExitCode()
}

func (s *CoverageTotalTestSuite) path(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(s.dir, p)
}

func (s *CoverageTotalTestSuite) TestTheTotalCountsWhatTheBinaryRan(t *gotest.T) {
	out, code := s.total(t, "suites.out", "data", "0", "covmod/...")
	gotest.Equal(t, 0, code, "output: %s", out)
	gotest.Equal(t, "coverage: 100.0% with the CLI's subprocess runs, 33.3% from the suites alone (minimum 0%)\n", out)
}

func (s *CoverageTotalTestSuite) TestATotalBelowTheMinimumFails(t *gotest.T) {
	out, code := s.total(t, "suites.out", "data", "100", "covmod/cmd/...")
	gotest.Equal(t, 1, code, "output: %s", out)
	gotest.Contains(t, out, "FAIL: 66.7% coverage with the CLI's subprocess runs (minimum 100%)")
}

func (s *CoverageTotalTestSuite) TestADirectoryWithoutDataIsRefused(t *gotest.T) {
	out, code := s.total(t, "suites.out", t.TempDir(), "0", "covmod/...")
	gotest.Equal(t, 2, code)
	gotest.Contains(t, out, "no coverage data in ")
	gotest.Contains(t, out, ": set GOCOVERDIR for the run, and run a suite that starts the CLI")
}

func (s *CoverageTotalTestSuite) TestProfilesOfDifferentModesAreRefused(t *gotest.T) {
	out, code := s.total(t, "set.out", "data", "0", "covmod/...")
	gotest.Equal(t, 2, code)
	gotest.Contains(t, out, "the suites' profile is mode: set and the CLI's data is mode: atomic: run the suites with -race or -covermode=atomic")
}
