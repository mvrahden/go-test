package gotestgen_test

import (
	"path/filepath"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/pkg/gotest"
	"golang.org/x/tools/go/packages"
)

// UtilsTestSuite tests generator utility functions like DeterminePkgDir.
type UtilsTestSuite struct{}

func (s *UtilsTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *UtilsTestSuite) TestDeterminePkgDir(t *gotest.T) {
	t.When("various package configurations", func(w *gotest.T) {
		for sub, tC := range gotest.Each(w, []struct {
			Desc        string
			ModuleDir   string
			ModulePath  string
			PackagePath string
			PackageName string
			Expected    string
		}{
			{Desc: "PTest", ModuleDir: "/user/xyz/projects", ModulePath: "github.com/user_xyz/module-abc", PackagePath: "github.com/user_xyz/module-abc/pkg_def", PackageName: "pkg_def", Expected: "/user/xyz/projects/pkg_def"},
			{Desc: "PXTest", ModuleDir: "/user/xyz/projects", ModulePath: "github.com/user_xyz/module-abc", PackagePath: "github.com/user_xyz/module-abc/pkg_def_test", PackageName: "pkg_def_test", Expected: "/user/xyz/projects/pkg_def"},
			{Desc: "PTest with low version", ModuleDir: "/user/xyz/projects", ModulePath: "github.com/user_xyz/module-abc/v2", PackagePath: "github.com/user_xyz/module-abc/v2/pkg_def", PackageName: "pkg_def", Expected: "/user/xyz/projects/pkg_def"},
			{Desc: "PXTest with low version", ModuleDir: "/user/xyz/projects", ModulePath: "github.com/user_xyz/module-abc/v2", PackagePath: "github.com/user_xyz/module-abc/v2/pkg_def_test", PackageName: "pkg_def_test", Expected: "/user/xyz/projects/pkg_def"},
			{Desc: "PTest with higher version", ModuleDir: "/user/xyz/projects", ModulePath: "github.com/user_xyz/module-abc/v99", PackagePath: "github.com/user_xyz/module-abc/v99/pkg_def", PackageName: "pkg_def", Expected: "/user/xyz/projects/pkg_def"},
			{Desc: "PXTest with higher version", ModuleDir: "/user/xyz/projects", ModulePath: "github.com/user_xyz/module-abc/v99", PackagePath: "github.com/user_xyz/module-abc/v99/pkg_def_test", PackageName: "pkg_def_test", Expected: "/user/xyz/projects/pkg_def"},
			{Desc: "PTest at module root", ModuleDir: "/user/xyz/projects", ModulePath: "github.com/user_xyz/module-abc", PackagePath: "github.com/user_xyz/module-abc", PackageName: "module_abc", Expected: "/user/xyz/projects"},
			{Desc: "PXTest at module root", ModuleDir: "/user/xyz/projects", ModulePath: "github.com/user_xyz/module-abc", PackagePath: "github.com/user_xyz/module-abc_test", PackageName: "module_abc_test", Expected: "/user/xyz/projects"},
		}) {
			actual := gotestgen.DeterminePkgDir(&packages.Package{Module: &packages.Module{Dir: tC.ModuleDir, Path: tC.ModulePath}, PkgPath: tC.PackagePath, Name: tC.PackageName})
			gotest.Equal(sub, tC.Expected, filepath.ToSlash(actual))
		}
	})
}
