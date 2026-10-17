package main_test

import (
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/gotestcli"
)

// MigrateCLITestSuite drives "gotest migrate" through the built binary: its
// exit code is the answer to "is anything left to do by hand", and
// --dry-run shows the edit without making it.
//
//nolint:lifecycle-pair // BeforeAll only wraps the shared binary, which its fixture removes
type MigrateCLITestSuite struct {
	CLI *gotestcli.BinarySharedFixture
	Mod *gotestcli.TestifyModuleSharedFixture
	cli cliRunner
}

func (s *MigrateCLITestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *MigrateCLITestSuite) BeforeAll(t *gotest.T) {
	s.cli = newCLIRunner(s.CLI)
}

type migrateCtx struct {
	dir  string
	path string
}

const migrateCleanSrc = `package sample

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type CleanSuite struct {
	suite.Suite
}

func (s *CleanSuite) TestOK() {
	s.Require().Equal(1, 1)
}

func TestCleanSuite(t *testing.T) {
	suite.Run(t, new(CleanSuite))
}
`

const migrateMarkedSrc = `package sample

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type MarkedSuite struct {
	suite.Suite
}

func (s *MarkedSuite) TestOK() {
	var target error
	s.Require().ErrorAs(nil, &target)
}

func TestMarkedSuite(t *testing.T) {
	suite.Run(t, new(MarkedSuite))
}
`

// BeforeEach makes a module of testify and the gotest under test: migrate
// type-checks what it converts.
func (s *MigrateCLITestSuite) BeforeEach(t *gotest.T) *migrateCtx {
	dir := t.TempDir()
	gotest.NoError(t, s.Mod.Write(dir, nil))
	return &migrateCtx{dir: dir, path: filepath.Join(dir, "sample_test.go")}
}

func (s *MigrateCLITestSuite) write(t *gotest.T, ctx *migrateCtx, src string) {
	gotest.NoError(t, os.WriteFile(ctx.path, []byte(src), 0o600))
}

func (s *MigrateCLITestSuite) read(t *gotest.T, ctx *migrateCtx) string {
	got, err := os.ReadFile(ctx.path)
	gotest.NoError(t, err)
	return string(got)
}

func (s *MigrateCLITestSuite) TestExitCode(t *gotest.T, ctx *migrateCtx) {
	t.When("everything converted", func(w *gotest.T) {
		s.write(w, ctx, migrateCleanSrc)
		out, code := runGotestIn(w, s.cli.binary, ctx.dir, nil, "migrate", ".")
		w.It("exits 0", func(it *gotest.T) {
			gotest.Equal(it, 0, code, out)
			gotest.Contains(it, out, "CleanSuite → CleanTestSuite")
		})
	})

	t.When("a marker was left", func(w *gotest.T) {
		s.write(w, ctx, migrateMarkedSrc)
		out, code := runGotestIn(w, s.cli.binary, ctx.dir, nil, "migrate", ".")
		w.It("exits 1 and counts the markers", func(it *gotest.T) {
			gotest.Equal(it, 1, code, out)
			gotest.Contains(it, out, "1 TODO(gotest-migrate) marker")
			gotest.Contains(it, s.read(it, ctx), "TODO(gotest-migrate)")
		})
	})
}

func (s *MigrateCLITestSuite) TestRerun(t *gotest.T, ctx *migrateCtx) {
	s.write(t, ctx, migrateMarkedSrc)
	_, _, first := s.cli.runSplitIn(t, ctx.dir, "migrate", ".")
	gotest.Equal(t, 1, first)
	migrated := s.read(t, ctx)

	stdout, stderr, code := s.cli.runSplitIn(t, ctx.dir, "migrate", ".")

	t.It("still exits 1 for the marker the first run left", func(it *gotest.T) {
		gotest.Equal(it, 1, code, stdout+stderr)
	})

	t.It("reports the file and its markers", func(it *gotest.T) {
		gotest.Equal(it, "Markers an earlier run left, still to resolve:\n  sample_test.go: 1 marker\n1 TODO(gotest-migrate) marker left; resolve them by hand\n", stdout)
		gotest.Empty(it, stderr)
	})

	t.It("does not touch the file again", func(it *gotest.T) {
		gotest.Equal(it, migrated, s.read(it, ctx))
	})
}

func (s *MigrateCLITestSuite) TestSiblingReference(t *gotest.T, ctx *migrateCtx) {
	s.write(t, ctx, migrateCleanSrc)
	sibling := filepath.Join(ctx.dir, "helper_test.go")
	gotest.NoError(t, os.WriteFile(sibling, []byte("package sample\n\nfunc use(s *CleanSuite) {}\n"), 0o600))

	stdout, stderr, code := s.cli.runSplitIn(t, ctx.dir, "migrate", ".")

	t.It("lists the file whose references it renamed", func(it *gotest.T) {
		gotest.Equal(it, 0, code, stdout+stderr)
		gotest.Contains(it, stdout, "Migrated 1 suites:\n  helper_test.go: updated for suites other files declare\n  sample_test.go: CleanSuite → CleanTestSuite\n")
		gotest.Empty(it, stderr)
	})

	t.It("renames the reference", func(it *gotest.T) {
		got, err := os.ReadFile(sibling)
		gotest.NoError(it, err)
		gotest.Contains(it, string(got), "func use(s *CleanTestSuite) {}")
	})
}

// migrateSplit spreads one suite over three files: the struct, its hooks
// and test, and its runner.
var migrateSplit = map[string]string{
	"suite_test.go":   "package sample\n\nimport \"github.com/stretchr/testify/suite\"\n\ntype SplitSuite struct {\n\tsuite.Suite\n\tn int\n}\n",
	"methods_test.go": "package sample\n\nfunc (s *SplitSuite) SetupTest() { s.n = 1 }\n\nfunc (s *SplitSuite) TestOK() {\n\ts.Require().Equal(1, s.n)\n}\n",
	"runner_test.go":  "package sample\n\nimport (\n\t\"testing\"\n\n\t\"github.com/stretchr/testify/suite\"\n)\n\nfunc TestSplitSuite(t *testing.T) {\n\tsuite.Run(t, new(SplitSuite))\n}\n",
}

func (s *MigrateCLITestSuite) TestSiblingFiles(t *gotest.T, ctx *migrateCtx) {
	for name, src := range migrateSplit {
		gotest.NoError(t, os.WriteFile(filepath.Join(ctx.dir, name), []byte(src), 0o600))
	}

	stdout, stderr, code := s.cli.runSplitIn(t, ctx.dir, "migrate", ".")
	read := func(it *gotest.T, name string) string {
		got, err := os.ReadFile(filepath.Join(ctx.dir, name))
		gotest.NoError(it, err)
		return string(got)
	}

	t.It("converts the suite wherever its parts are declared and exits 0", func(it *gotest.T) {
		gotest.Equal(it, 0, code, stdout+stderr)
		gotest.Equal(it, "Migrated 1 suites:\n  methods_test.go: updated for suites other files declare\n  runner_test.go: updated for suites other files declare\n  suite_test.go: SplitSuite → SplitTestSuite\n", stdout)
		gotest.Empty(it, stderr)
	})

	t.It("renames the hook and gives the test t", func(it *gotest.T) {
		methods := read(it, "methods_test.go")
		gotest.Contains(it, methods, "func (s *SplitTestSuite) BeforeEach(t *gotest.T) { s.n = 1 }")
		gotest.Contains(it, methods, "func (s *SplitTestSuite) TestOK(t *gotest.T) {\n\tgotest.Equal(t, 1, s.n)")
	})

	t.It("removes the runner from its file", func(it *gotest.T) {
		gotest.Equal(it, "package sample\n", read(it, "runner_test.go"))
	})
}

func (s *MigrateCLITestSuite) TestOverlappingPatterns(t *gotest.T, ctx *migrateCtx) {
	s.write(t, ctx, migrateCleanSrc)

	stdout, stderr, code := s.cli.runSplitIn(t, ctx.dir, "migrate", "--dry-run", ".", "./...")

	t.It("migrates the file both patterns name once", func(it *gotest.T) {
		gotest.Equal(it, 0, code, stdout+stderr)
		gotest.Equal(it, 1, strings.Count(stdout, "+++ b/sample_test.go"))
		gotest.Equal(it, 1, strings.Count(stdout, "CleanSuite → CleanTestSuite"))
		gotest.Empty(it, stderr)
	})
}

func (s *MigrateCLITestSuite) TestDryRun(t *gotest.T, ctx *migrateCtx) {
	s.write(t, ctx, migrateMarkedSrc)
	out, code := runGotestIn(t, s.cli.binary, ctx.dir, nil, "migrate", "--dry-run", ".")

	t.It("prints the diff and the markers it would leave", func(it *gotest.T) {
		gotest.Contains(it, out, "--- a/sample_test.go")
		gotest.Contains(it, out, "+++ b/sample_test.go")
		gotest.Contains(it, out, "+type MarkedTestSuite struct {")
		gotest.Contains(it, out, "+\t// TODO(gotest-migrate): unmapped assertion ErrorAs")
	})

	t.It("writes nothing", func(it *gotest.T) {
		gotest.Equal(it, migrateMarkedSrc, s.read(it, ctx))
	})

	t.It("still exits 1 for the markers", func(it *gotest.T) {
		gotest.Equal(it, 1, code, out)
	})
}

func (s *MigrateCLITestSuite) TestDryRunApplies(t *gotest.T, ctx *migrateCtx) {
	if runtime.GOOS == "windows" {
		t.Skipf("symlinks need privileges on Windows")
	}
	// The suite file lacks a trailing newline; the second is a symlink to
	// a file in another directory of the module.
	s.write(t, ctx, strings.TrimSuffix(migrateMarkedSrc, "\n"))
	gotest.NoError(t, os.MkdirAll(filepath.Join(ctx.dir, "real"), 0o750))
	gotest.NoError(t, os.WriteFile(filepath.Join(ctx.dir, "real", "linked.go.txt"), []byte(migrateCleanSrc), 0o600))
	gotest.NoError(t, os.Symlink(filepath.Join("real", "linked.go.txt"), filepath.Join(ctx.dir, "linked_test.go")))
	written := t.TempDir()
	gotest.NoError(t, os.CopyFS(written, os.DirFS(ctx.dir)))
	_, _, _ = s.cli.runSplitIn(t, written, "migrate", ".")

	patch, stderr, code := s.cli.runSplitIn(t, ctx.dir, "migrate", "--dry-run", ".")
	gotest.Equal(t, 1, code, stderr)
	patch, _, _ = strings.Cut(patch, "Would migrate")
	apply := exec.CommandContext(t.Context(), "git", "apply", "-")
	apply.Dir, apply.Stdin = ctx.dir, strings.NewReader(patch)
	out, err := apply.CombinedOutput()

	t.It("applies with git apply and leaves what migrate writes", func(it *gotest.T) {
		gotest.NoError(it, err, string(out)+patch)
		for _, name := range []string{"sample_test.go", filepath.Join("real", "linked.go.txt")} {
			want, err := os.ReadFile(filepath.Join(written, name))
			gotest.NoError(it, err)
			gotest.Equal(it, string(want), string(gotest.Must(os.ReadFile(filepath.Join(ctx.dir, name)))), name)
		}
	})
	t.It("keeps the symlink", func(it *gotest.T) {
		info, err := os.Lstat(filepath.Join(ctx.dir, "linked_test.go"))
		gotest.NoError(it, err)
		gotest.NotZero(it, info.Mode()&os.ModeSymlink)
	})
}

// module lays out the module example.com/m with a suite in pkg and one in
// a testdata directory, which no package pattern names.
func (s *MigrateCLITestSuite) module(t *gotest.T, ctx *migrateCtx) {
	for name, src := range map[string]string{
		"pkg/sample_test.go":           migrateCleanSrc,
		"pkg/testdata/fixture_test.go": migrateCleanSrc,
	} {
		path := filepath.Join(ctx.dir, name)
		gotest.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		gotest.NoError(t, os.WriteFile(path, []byte(src), 0o600))
	}
}

func (s *MigrateCLITestSuite) TestImportPathPattern(t *gotest.T, ctx *migrateCtx) {
	s.module(t, ctx)

	for _, pattern := range []string{"example.com/m/pkg", "example.com/m/..."} {
		t.When("the pattern is "+pattern, func(w *gotest.T) {
			stdout, stderr, code := s.cli.runSplitIn(w, ctx.dir, "migrate", "--dry-run", pattern)

			w.It("migrates the package it names", func(it *gotest.T) {
				gotest.Equal(it, 0, code, stdout+stderr)
				gotest.Contains(it, stdout, "Would migrate 1 suites:\n  "+filepath.FromSlash("pkg/sample_test.go")+": CleanSuite → CleanTestSuite\n")
				gotest.Empty(it, stderr)
			})
		})
	}

	t.When("the import path does not exist", func(w *gotest.T) {
		stdout, stderr, code := s.cli.runSplitIn(w, ctx.dir, "migrate", "example.com/m/nope")

		w.It("fails as a load error, naming the pattern", func(it *gotest.T) {
			gotest.Equal(it, 2, code, stdout+stderr)
			gotest.Regexp(it, `^migrate: pattern example\.com/m/nope: `, stderr)
			gotest.Empty(it, stdout)
		})
	})
}

func (s *MigrateCLITestSuite) TestTestdataSkipped(t *gotest.T, ctx *migrateCtx) {
	s.module(t, ctx)

	stdout, stderr, code := s.cli.runSplitIn(t, ctx.dir, "migrate", "./...")

	t.It("migrates the packages ./... names", func(it *gotest.T) {
		gotest.Equal(it, 0, code, stdout+stderr)
		gotest.Equal(it, "Migrated 1 suites:\n  "+filepath.FromSlash("pkg/sample_test.go")+": CleanSuite → CleanTestSuite\n", stdout)
		gotest.Empty(it, stderr)
	})

	t.It("leaves a testdata fixture alone", func(it *gotest.T) {
		got, err := os.ReadFile(filepath.Join(ctx.dir, "pkg", "testdata", "fixture_test.go"))
		gotest.NoError(it, err)
		gotest.Equal(it, migrateCleanSrc, string(got))
	})
}

func (s *MigrateCLITestSuite) TestMissingPattern(t *gotest.T, ctx *migrateCtx) {
	stdout, stderr, code := s.cli.runSplitIn(t, ctx.dir, "migrate", "./nonexistent")

	t.It("fails as a load error", func(it *gotest.T) {
		gotest.Equal(it, 2, code, stdout+stderr)
	})

	t.It("names the pattern on stderr", func(it *gotest.T) {
		gotest.Regexp(it, `^migrate: pattern \./nonexistent: `, stderr)
	})

	t.It("does not claim the pattern was clean", func(it *gotest.T) {
		gotest.Empty(it, stdout)
	})
}

func (s *MigrateCLITestSuite) TestUncompiledPackage(t *gotest.T, ctx *migrateCtx) {
	src := strings.Replace(migrateCleanSrc, "s.Require().Equal(1, 1)", "undefined()", 1)
	s.write(t, ctx, src)

	stdout, stderr, code := s.cli.runSplitIn(t, ctx.dir, "migrate", ".")

	t.It("exits 2: nothing about a migration could be proven", func(it *gotest.T) {
		gotest.Equal(it, 2, code, stdout+stderr)
	})

	t.It("names the package and its error", func(it *gotest.T) {
		gotest.Equal(it, "Not migrated: these packages do not compile as they are, so nothing about a migration could be proven:\n  .:\n    sample_test.go:14: undefined: undefined\n", stdout)
		gotest.Empty(it, stderr)
	})

	t.It("leaves the file untouched", func(it *gotest.T) {
		gotest.Equal(it, src, s.read(it, ctx))
	})
}

func (s *MigrateCLITestSuite) TestUnwritableFile(t *gotest.T, ctx *migrateCtx) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skipf("POSIX file permissions as a non-root user")
	}
	for name, src := range migrateSplit {
		gotest.NoError(t, os.WriteFile(filepath.Join(ctx.dir, name), []byte(src), 0o600))
	}
	// The second of the three files migrate writes.
	gotest.NoError(t, os.Chmod(filepath.Join(ctx.dir, "runner_test.go"), 0o400))
	before := listDir(t, ctx.dir)

	stdout, stderr, code := s.cli.runSplitIn(t, ctx.dir, "migrate", ".")

	t.It("exits 2 and says nothing was written", func(it *gotest.T) {
		gotest.Equal(it, 2, code, stdout+stderr)
		gotest.Regexp(it, `^migrate: writing the migrated files: open \S*runner_test\.go: permission denied; nothing was written\n$`, stderr)
		gotest.Empty(it, stdout)
	})

	t.It("writes none of the files and leaves no temporary file", func(it *gotest.T) {
		for name, src := range migrateSplit {
			got, err := os.ReadFile(filepath.Join(ctx.dir, name))
			gotest.NoError(it, err)
			gotest.Equal(it, src, string(got), name)
		}
		gotest.Equal(it, before, listDir(it, ctx.dir))
	})
}

const migrateRawStringSrc = "package sample\n\nimport (\n\t\"testing\"\n\n\t\"github.com/stretchr/testify/suite\"\n)\n\ntype RawSuite struct {\n\tsuite.Suite\n}\n\nfunc (s *RawSuite) TestRx() {\n\tbody := []byte(\"ab\")\n\ts.Require().Regexp(`a\nb`, body)\n}\n\nfunc TestRawSuite(t *testing.T) {\n\tsuite.Run(t, new(RawSuite))\n}\n"

func (s *MigrateCLITestSuite) TestMarkerBesideRawString(t *gotest.T, ctx *migrateCtx) {
	s.write(t, ctx, migrateRawStringSrc)

	stdout, stderr, code := s.cli.runSplitIn(t, ctx.dir, "migrate", ".")
	got := s.read(t, ctx)

	t.It("exits 1: the error on a line inside the literal is marked", func(it *gotest.T) {
		gotest.Equal(it, 1, code, stdout+stderr)
		gotest.Contains(it, stdout, "1 TODO(gotest-migrate) marker left")
	})

	t.It("leaves the literal as it was", func(it *gotest.T) {
		gotest.Contains(it, got, "gotest.Regexp(t, `a\nb`, body)")
	})

	t.It("places the marker as a comment above the statement", func(it *gotest.T) {
		f, err := parser.ParseFile(token.NewFileSet(), "", got, parser.ParseComments)
		gotest.NoError(it, err)
		var comments []string
		for _, cg := range f.Comments {
			comments = append(comments, cg.Text())
		}
		gotest.Len(it, comments, 1)
		gotest.Regexp(it, `^TODO\(gotest-migrate\): does not compile: cannot use body`, comments[0])
		gotest.Contains(it, got, "\t// TODO(gotest-migrate): does not compile: cannot use body (variable of type []byte) as string value in argument to gotest.Regexp\n\tgotest.Regexp(")
	})
}

// migrateGenerated has a generated suite, a generated runner of a suite
// declared by hand, a generated file using testify without its suites, and
// a suite nothing generated touches.
var migrateGenerated = map[string]string{
	"gen_test.go":  "// Code generated by suitegen. DO NOT EDIT.\n\npackage sample\n\nimport (\n\t\"testing\"\n\n\t\"github.com/stretchr/testify/suite\"\n)\n\ntype GenSuite struct{ suite.Suite }\n\nfunc (s *GenSuite) TestG() { s.Require().True(true) }\n\nfunc TestGen(t *testing.T) { suite.Run(t, new(GenSuite)) }\n",
	"genb_test.go": "// Code generated by runnergen. DO NOT EDIT.\n\npackage sample\n\nimport (\n\t\"testing\"\n\n\t\"github.com/stretchr/testify/suite\"\n)\n\nfunc TestB(t *testing.T) { suite.Run(t, new(BSuite)) }\n",
	"gena_test.go": "// Code generated by assertgen. DO NOT EDIT.\n\npackage sample\n\nimport \"github.com/stretchr/testify/assert\"\n\nvar _ = assert.True\n",
	"b_test.go":    "package sample\n\nimport \"github.com/stretchr/testify/suite\"\n\ntype BSuite struct{ suite.Suite }\n\nfunc (s *BSuite) TestB() { s.Require().True(true) }\n",
}

func (s *MigrateCLITestSuite) TestGeneratedFiles(t *gotest.T, ctx *migrateCtx) {
	s.write(t, ctx, migrateCleanSrc)
	for name, src := range migrateGenerated {
		gotest.NoError(t, os.WriteFile(filepath.Join(ctx.dir, name), []byte(src), 0o600))
	}
	read := func(it *gotest.T, name string) string {
		got, err := os.ReadFile(filepath.Join(ctx.dir, name))
		gotest.NoError(it, err)
		return string(got)
	}

	stdout, stderr, code := s.cli.runSplitIn(t, ctx.dir, "migrate", ".")

	t.It("names the generated files on stderr as skipped", func(it *gotest.T) {
		gotest.Equal(it, "Skipped, generated (Code generated ... DO NOT EDIT.); migrate their generator:\n  gen_test.go\n  genb_test.go\n", stderr)
	})

	t.It("leaves them as they were", func(it *gotest.T) {
		gotest.Equal(it, migrateGenerated["gen_test.go"], read(it, "gen_test.go"))
		gotest.Equal(it, migrateGenerated["genb_test.go"], read(it, "genb_test.go"))
	})

	t.It("refuses the suite a generated file uses, and migrates the rest", func(it *gotest.T) {
		gotest.Equal(it, 1, code, stdout+stderr)
		gotest.Contains(it, stdout, "sample_test.go: CleanSuite → CleanTestSuite\n")
		gotest.Contains(it, read(it, "b_test.go"), "// TODO(gotest-migrate): BSuite is used in genb_test.go, which is generated — migrate it by hand, then regenerate that file\ntype BSuite struct{ suite.Suite }\n")
	})
}
