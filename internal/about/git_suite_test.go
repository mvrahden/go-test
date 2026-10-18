package about_test

import (
	"go/ast"
	"go/token"
	"path/filepath"

	"github.com/mvrahden/go-test/internal/about"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// GitTestSuite tests PSuiteRegex pattern matching for generated suite file discovery.
type GitTestSuite struct{}

func (s *GitTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *GitTestSuite) TestPSuiteRegex(t *gotest.T) {
	t.When("generated suite filenames", func(w *gotest.T) {
		for sub, tc := range gotest.Each(w, []struct{ Name string }{
			{"gotest_psuite_test.go"},
			{"gotest_pxsuite_test.go"},
			{"gosuite/gotest_psuite_test.go"},
			{"gosuite/gotest_pxsuite_test.go"},
			{"ƒƒ_psuite_test.go"},
			{"ƒƒ_pxsuite_test.go"},
		}) {
			gotest.Regexp(sub, about.PSuiteRegex, tc.Name)
		}
	})

	t.When("non-suite filenames", func(w *gotest.T) {
		for sub, tc := range gotest.Each(w, []struct{ Name string }{
			{"ptest_test.go"},
			{"pxtest_test.go"},
			{"focus_suite/main.go"},
			{"simple_suite/ptest_test.go"},
			{"simple_suite/pxtest_test.go"},
			{"focus_suite/gotestgen_ptest.golden"},
			{"focus_suite/gotestgen_pxtest.golden"},
			{"ƒƒ_suite_test.go"},
		}) {
			gotest.False(sub, about.PSuiteRegex.MatchString(tc.Name))
		}
	})
}

func (s *GitTestSuite) TestParseStubbingGenerated(t *gotest.T) {
	src := []byte("package p\n\nimport \"testing\"\n\nfunc TestOld(t *testing.T) { _ = &OldTestSuite{} }\n")
	t.When("a generated file is parsed", func(w *gotest.T) {
		for sub, tc := range gotest.Each(w, []struct{ Name string }{
			{about.PSuite}, {about.PXSuite}, {about.LegacyPSuite}, {about.LegacyPXSuite},
		}) {
			name := filepath.Join("pkg", tc.Name)
			fset := token.NewFileSet()
			f, err := about.ParseStubbingGenerated(fset, name, src)
			gotest.NoError(sub, err)
			gotest.Equal(sub, "p", f.Name.Name)
			gotest.Empty(sub, f.Decls)
			gotest.Equal(sub, name, fset.File(f.Pos()).Name())
		}
	})
	t.When("any other file is parsed", func(w *gotest.T) {
		f, err := about.ParseStubbingGenerated(token.NewFileSet(), "pkg/p_test.go", src)
		gotest.NoError(w, err)
		gotest.Len(w, f.Decls, 2)
		_, isFunc := f.Decls[1].(*ast.FuncDecl)
		gotest.True(w, isFunc)
	})
}

func (s *GitTestSuite) TestGeneratedFileNames(t *gotest.T) {
	gotest.ElementsMatch(t, []string{"gotest_psuite_test.go", "gotest_pxsuite_test.go", "ƒƒ_psuite_test.go", "ƒƒ_pxsuite_test.go"}, about.GeneratedFileNames)
}

func (s *GitTestSuite) TestPackageStub(t *gotest.T) {
	t.When("the file has a build constraint", func(w *gotest.T) {
		src := []byte("//go:build ignore\n\npackage main\n\nfunc main() {}\n")
		gotest.Equal(w, "//go:build ignore\n\npackage main\n", string(about.PackageStub(src)))
	})
	t.When("the file has no package clause", func(w *gotest.T) {
		gotest.Empty(w, about.PackageStub([]byte("not go")))
	})
}
