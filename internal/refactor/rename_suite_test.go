package refactor_test

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mvrahden/go-test/internal/refactor"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// RenameTestSuite toggles focus on testdata packages and holds every file
// to its golden: each reference to the suite or method is renamed, nothing
// else is, and the package still compiles both ways.
type RenameTestSuite struct{}

func (s *RenameTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

type renameCase struct {
	Desc  string
	Pkg   string // under testdata
	File  string // the file holding the declaration
	Ident string // Suite or Suite.Method, unfocused
}

func (s *RenameTestSuite) TestToggleFocus(t *gotest.T) {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	cases := []renameCase{
		{Desc: "a suite whose methods sit in several files", Pkg: "multifile", File: "suite_test.go", Ident: "MultiTestSuite"},
		{Desc: "a method another file refers to", Pkg: "multifile", File: "cases_test.go", Ident: "MultiTestSuite.TestTwo"},
		{Desc: "a generic suite that aliases instantiate", Pkg: "generic", File: "contract_test.go", Ident: "ContractTestSuite"},
		{Desc: "a generic method an alias refers to", Pkg: "generic", File: "more_test.go", Ident: "ContractTestSuite.TestCopy"},
		{Desc: "a suite used in literals and other expressions", Pkg: "uses", File: "suite_test.go", Ident: "UsesTestSuite"},
		{Desc: "a suite whose name other scopes reuse", Pkg: "shadow", File: "suite_test.go", Ident: "ShadowTestSuite"},
		{Desc: "a method whose name other types reuse", Pkg: "shadow", File: "suite_test.go", Ident: "ShadowTestSuite.TestOne"},
	}
	for _, c := range cases {
		t.It(c.Desc, func(it *gotest.T) { s.toggle(it, root, c) })
	}
}

// toggle focuses c's identifier in a staged copy of its package and back.
func (s *RenameTestSuite) toggle(t *gotest.T, root string, c renameCase) {
	src := filepath.Join("testdata", c.Pkg)
	mod := t.TempDir()
	dir, err := testkit.StagePackage(root, src, mod, "refactormod")
	gotest.NoError(t, err)
	original := goFiles(t, dir)

	gotest.NoError(t, refactor.ToggleFocus(filepath.Join(dir, c.File), c.Ident))
	vet(t, mod)
	golden := filepath.Join("testdata", c.Pkg+".golden", c.Ident)
	for rel, before := range original {
		want := before
		if g, err := os.ReadFile(filepath.Join(golden, rel+".golden")); err == nil {
			want = string(g)
		}
		gotest.Equal(t, want, readFile(t, filepath.Join(dir, rel)), "file %s", rel)
	}

	gotest.NoError(t, refactor.ToggleFocus(filepath.Join(dir, c.File), focused(c.Ident)))
	vet(t, mod)
	gotest.Equal(t, original, goFiles(t, dir))
}

// focused returns ident with the F_ prefix on its last part.
func focused(ident string) string {
	if suite, method, ok := strings.Cut(ident, "."); ok {
		return suite + ".F_" + method
	}
	return "F_" + ident
}

// goFiles returns the Go files under dir by their slash path relative to it.
func goFiles(t *gotest.T, dir string) map[string]string {
	files := map[string]string{}
	gotest.NoError(t, filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err == nil {
			files[filepath.ToSlash(rel)] = readFile(t, path)
		}
		return err
	}))
	return files
}

func readFile(t *gotest.T, path string) string {
	b, err := os.ReadFile(path)
	gotest.NoError(t, err)
	return string(b)
}

// vet type-checks every package of the module at mod, test files included.
func vet(t *gotest.T, mod string) {
	cmd := exec.CommandContext(t.Context(), "go", "vet", "./...")
	cmd.Dir = mod
	out, err := cmd.CombinedOutput()
	gotest.NoError(t, err, string(out))
}
