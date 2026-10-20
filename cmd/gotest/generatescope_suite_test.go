package main_test

import (
	"os"
	"path/filepath"
	"strconv"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/gotestcli"
)

// GenerateScopeTestSuite runs generate and clean over a main module that
// imports a replaced dependency with suites of its own: files are written
// and removed in main modules only, a run still runs the dependency. The
// dependency is named, not reached through "all": that loads every package
// gotest itself depends on, minutes on a cold CI cache.
//
//nolint:lifecycle-pair // BeforeAll only wraps the shared binary, which its fixture removes
type GenerateScopeTestSuite struct {
	CLI *gotestcli.BinarySharedFixture
	cli cliRunner
}

func (s *GenerateScopeTestSuite) SuiteConfig() gotest.SuiteConfig {
	// No deadline: this suite compiles code, so its time is not a verdict.
	return gotest.SuiteConfig{Parallel: true, Timeout: gotest.NoDeadline}
}

func (s *GenerateScopeTestSuite) BeforeAll(t *gotest.T) {
	s.cli = newCLIRunner(s.CLI)
}

type scopeLayout struct {
	main, dep string
}

const depNote = "note: skipped example.com/dep: not in a main module\n"

// stage writes dep/ and main/ under a fresh directory; main imports dep
// through a replace. With workspace, a go.work at the top uses both.
func (s *GenerateScopeTestSuite) stage(t *gotest.T, workspace bool) scopeLayout {
	root := t.TempDir()
	l := scopeLayout{main: filepath.Join(root, "main"), dep: filepath.Join(root, "dep")}
	gotestReq := "require github.com/mvrahden/go-test v0.0.0-00010101000000-000000000000\n\nreplace github.com/mvrahden/go-test => " + s.cli.repoRoot + "\n"
	files := map[string]string{
		filepath.Join(l.dep, "go.mod"):      "module example.com/dep\n\ngo 1.26.0\n\n" + gotestReq,
		filepath.Join(l.dep, "dep.go"):      "package dep\n\nfunc Two() int { return 2 }\n",
		filepath.Join(l.dep, "dep_test.go"): suiteSource("dep", "Dep", "Two()", 2),
		filepath.Join(l.main, "go.mod"): "module example.com/main\n\ngo 1.26.0\n\n" + gotestReq +
			"\nrequire example.com/dep v0.0.0\n\nreplace example.com/dep => ../dep\n",
		filepath.Join(l.main, "main.go"):      "package main\n\nimport \"example.com/dep\"\n\nfunc one() int { return dep.Two() - 1 }\n\nfunc main() {}\n",
		filepath.Join(l.main, "main_test.go"): suiteSource("main", "Main", "one()", 1),
	}
	if workspace {
		files[filepath.Join(root, "go.work")] = "go 1.26.0\n\nuse (\n\t./main\n\t./dep\n)\n"
	}
	for path, src := range files {
		gotest.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
		gotest.NoError(t, os.WriteFile(path, []byte(src), 0o600))
	}
	return l
}

func suiteSource(pkg, name, call string, want int) string {
	return "package " + pkg + "\n\nimport \"github.com/mvrahden/go-test/pkg/gotest\"\n\n" +
		"type " + name + "TestSuite struct{}\n\n" +
		"func (s *" + name + "TestSuite) TestValue(t *gotest.T) { gotest.Equal(t, " + strconv.Itoa(want) + ", " + call + ") }\n"
}

func generated(dir string) string { return filepath.Join(dir, "gotest_psuite_test.go") }

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (s *GenerateScopeTestSuite) TestGenerateWritesIntoTheMainModuleOnly(t *gotest.T) {
	l := s.stage(t, false)
	stdout, stderr, code := s.cli.runSplitIn(t, l.main, "generate", "./...", "example.com/dep")

	t.It("writes the main module's file", func(it *gotest.T) {
		gotest.Equal(it, 0, code, stderr)
		gotest.Equal(it, generated(l.main)+"\n", stdout)
		gotest.True(it, exists(generated(l.main)))
	})
	t.It("leaves the dependency untouched and says so", func(it *gotest.T) {
		gotest.False(it, exists(generated(l.dep)))
		gotest.Contains(it, stderr, depNote)
	})
}

func (s *GenerateScopeTestSuite) TestCleanRemovesWhatGenerateWrote(t *gotest.T) {
	l := s.stage(t, false)
	_, stderr, code := s.cli.runSplitIn(t, l.main, "generate", "./...", "example.com/dep")
	gotest.Equal(t, 0, code, stderr)
	// As an earlier gotest left it in the dependency.
	gotest.NoError(t, os.WriteFile(generated(l.dep), []byte("package dep\n"), 0o600))

	stdout, stderr, code := s.cli.runSplitIn(t, l.main, "clean", "./...", "example.com/dep")

	t.It("removes the main module's file", func(it *gotest.T) {
		gotest.Equal(it, 0, code, stderr)
		gotest.Equal(it, generated(l.main)+"\n", stdout)
		gotest.False(it, exists(generated(l.main)))
	})
	t.It("leaves the dependency's file and says so", func(it *gotest.T) {
		gotest.True(it, exists(generated(l.dep)))
		gotest.Equal(it, depNote+"removed 1 generated file(s)\n", stderr)
	})
}

func (s *GenerateScopeTestSuite) TestAWorkspaceModuleIsAMainModule(t *gotest.T) {
	l := s.stage(t, true)

	_, stderr, code := s.cli.runSplitIn(t, l.main, "generate", "./...", "example.com/dep")
	gotest.Equal(t, 0, code, stderr)
	gotest.NotContains(t, stderr, depNote)
	gotest.True(t, exists(generated(l.main)))
	gotest.True(t, exists(generated(l.dep)))

	_, stderr, code = s.cli.runSplitIn(t, l.main, "clean", "./...", "example.com/dep")
	gotest.Equal(t, 0, code, stderr)
	gotest.False(t, exists(generated(l.main)))
	gotest.False(t, exists(generated(l.dep)))
}

func (s *GenerateScopeTestSuite) TestARunStillRunsTheDependencysSuites(t *gotest.T) {
	l := s.stage(t, false)
	stdout, stderr, code := s.cli.runSplitIn(t, l.main, "example.com/dep")

	gotest.Equal(t, 0, code, "stdout:\n%s\nstderr:\n%s", stdout, stderr)
	gotest.Regexp(t, `(?m)^ok  \texample\.com/dep\t`, stdout)
	gotest.False(t, exists(generated(l.dep)))
}
