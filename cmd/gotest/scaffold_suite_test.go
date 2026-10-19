package main_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/gotestcli"
)

// ScaffoldCommandTestSuite drives 'gotest scaffold' through the built binary
// against throwaway modules, so nothing it writes touches the repository.
//
//nolint:lifecycle-pair // BeforeAll only wraps the shared binary, which its fixture removes
type ScaffoldCommandTestSuite struct {
	CLI *gotestcli.BinarySharedFixture
	cli cliRunner
}

func (s *ScaffoldCommandTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *ScaffoldCommandTestSuite) BeforeAll(t *gotest.T) {
	s.cli = newCLIRunner(s.CLI)
}

const scaffoldGenericSrc = `package gen

type Index[T any] struct{ items []T }

func (i *Index[T]) Len() int { return len(i.items) }

type Store[K comparable] interface {
	Get(k K) (string, error)
}

func Ident[T any](v T) T { return v }

type Plain struct{}

func (p *Plain) Do() error { return nil }

func Render(n int) string { return "" }
`

// scaffoldRun is one scaffold invocation with its streams kept apart.
type scaffoldRun struct {
	stdout, stderr string
	exit           int
}

// stageScaffoldModule writes a module holding package gen and returns the
// module's directory and the package's.
func stageScaffoldModule(t *gotest.T, files map[string]string) (modDir, pkgDir string) {
	modDir = t.TempDir()
	pkgDir = filepath.Join(modDir, "gen")
	gotest.NoError(t, os.WriteFile(filepath.Join(modDir, "go.mod"), []byte("module example.com/scaf\n\ngo 1.24\n"), 0o600))
	gotest.NoError(t, os.MkdirAll(pkgDir, 0o750))
	for name, src := range files {
		gotest.NoError(t, os.WriteFile(filepath.Join(pkgDir, name), []byte(src), 0o600))
	}
	return modDir, pkgDir
}

func (s *ScaffoldCommandTestSuite) scaffold(t *gotest.T, dir string, args ...string) scaffoldRun {
	cmd := exec.Command(s.cli.binary, append([]string{"scaffold"}, args...)...) //nolint:gosec // G204: the built CLI with fixed args
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	var exitErr *exec.ExitError
	gotest.True(t, err == nil || errors.As(err, &exitErr), "running gotest: %v", err)
	return scaffoldRun{stdout: stdout.String(), stderr: stderr.String(), exit: cmd.ProcessState.ExitCode()}
}

// entries lists the names in dir.
func entries(t *gotest.T, dir string) []string {
	des, err := os.ReadDir(dir)
	gotest.NoError(t, err)
	names := make([]string, 0, len(des))
	for _, de := range des {
		names = append(names, de.Name())
	}
	return names
}

func (s *ScaffoldCommandTestSuite) TestGenericTargets(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct {
		Desc string
		args []string
		want string
	}{
		{"a generic struct", []string{"./gen.Index"}, "scaffold: Index is generic (type Index[T any])"},
		{"a generic interface", []string{"./gen.Store"}, "scaffold: Store is generic (type Store[K comparable])"},
		{"a generic function", []string{"--fuzz", "./gen.Ident"}, "scaffold: Ident is generic (func Ident[T any])"},
		{"a method of a generic type", []string{"./gen.Index.Len"}, "scaffold: Index.Len is a method of a generic type: Index is generic (type Index[T any])"},
	}) {
		modDir, pkgDir := stageScaffoldModule(sub, map[string]string{"gen.go": scaffoldGenericSrc})
		run := s.scaffold(sub, modDir, tc.args...)
		gotest.Equal(sub, 1, run.exit, "stdout: %s\nstderr: %s", run.stdout, run.stderr)
		gotest.Contains(sub, run.stderr, tc.want)
		gotest.Empty(sub, run.stdout)
		gotest.Equal(sub, []string{"gen.go"}, entries(sub, pkgDir), "scaffold wrote a file")
	}
}

func (s *ScaffoldCommandTestSuite) TestTakenSuiteName(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct {
		Desc     string
		testFile string
		testSrc  string
		want     string
	}{
		{"declared in the package's own test files", "plain_extra_test.go", "package gen\n\ntype PlainTestSuite struct{}\n", "type PlainTestSuite in package gen at "},
		{"declared in the external test package", "plain_extra_test.go", "package gen_test\n\nfunc PlainTestSuite() {}\n", "func PlainTestSuite in package gen_test at "},
	}) {
		modDir, pkgDir := stageScaffoldModule(sub, map[string]string{"gen.go": scaffoldGenericSrc, tc.testFile: tc.testSrc})
		run := s.scaffold(sub, modDir, "./gen.Plain")
		gotest.Equal(sub, 1, run.exit, "stdout: %s\nstderr: %s", run.stdout, run.stderr)
		gotest.Contains(sub, run.stderr, "scaffold: PlainTestSuite is already declared: "+tc.want+filepath.Join(pkgDir, tc.testFile)+":3")
		gotest.ElementsMatch(sub, []string{"gen.go", tc.testFile}, entries(sub, pkgDir), "scaffold wrote a file")
	}
}

func (s *ScaffoldCommandTestSuite) TestAbsolutePackagePaths(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct {
		Desc    string
		fuzz    bool
		target  string
		written string
	}{
		{"a type", false, "Plain", "plain_suite_test.go"},
		{"a function to fuzz", true, "Render", "render_fuzz_test.go"},
	}) {
		modDir, pkgDir := stageScaffoldModule(sub, map[string]string{"gen.go": scaffoldGenericSrc})
		args := []string{filepath.Join(modDir, "gen") + "." + tc.target}
		if tc.fuzz {
			args = append([]string{"--fuzz"}, args...)
		}
		// Run from outside the module: only the path finds it.
		run := s.scaffold(sub, sub.TempDir(), args...)
		gotest.Equal(sub, 0, run.exit, "stdout: %s\nstderr: %s", run.stdout, run.stderr)
		_, err := os.Stat(filepath.Join(pkgDir, tc.written))
		gotest.NoError(sub, err, "the scaffold was not written into the package's directory")
	}
}
