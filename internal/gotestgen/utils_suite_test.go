package gotestgen_test

import (
	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/pkg/gotest"
	"golang.org/x/tools/go/packages"
)

type UtilsTestSuite struct{}

func (s *UtilsTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *UtilsTestSuite) TestDeterminePkgDir(t *gotest.T) {
	t.When("various package configurations", func(w *gotest.T) {
		for _, tC := range []struct {
			desc        string
			ModuleDir   string
			ModulePath  string
			PackagePath string
			PackageName string
			Expected    string
		}{
			{desc: "PTest", ModuleDir: "/user/xyz/projects", ModulePath: "github.com/user_xyz/module-abc", PackagePath: "github.com/user_xyz/module-abc/pkg_def", PackageName: "pkg_def", Expected: "/user/xyz/projects/pkg_def"},
			{desc: "PXTest", ModuleDir: "/user/xyz/projects", ModulePath: "github.com/user_xyz/module-abc", PackagePath: "github.com/user_xyz/module-abc/pkg_def_test", PackageName: "pkg_def_test", Expected: "/user/xyz/projects/pkg_def"},
			{desc: "PTest with low version", ModuleDir: "/user/xyz/projects", ModulePath: "github.com/user_xyz/module-abc/v2", PackagePath: "github.com/user_xyz/module-abc/v2/pkg_def", PackageName: "pkg_def", Expected: "/user/xyz/projects/pkg_def"},
			{desc: "PXTest with low version", ModuleDir: "/user/xyz/projects", ModulePath: "github.com/user_xyz/module-abc/v2", PackagePath: "github.com/user_xyz/module-abc/v2/pkg_def_test", PackageName: "pkg_def_test", Expected: "/user/xyz/projects/pkg_def"},
			{desc: "PTest with higher version", ModuleDir: "/user/xyz/projects", ModulePath: "github.com/user_xyz/module-abc/v99", PackagePath: "github.com/user_xyz/module-abc/v99/pkg_def", PackageName: "pkg_def", Expected: "/user/xyz/projects/pkg_def"},
			{desc: "PXTest with higher version", ModuleDir: "/user/xyz/projects", ModulePath: "github.com/user_xyz/module-abc/v99", PackagePath: "github.com/user_xyz/module-abc/v99/pkg_def_test", PackageName: "pkg_def_test", Expected: "/user/xyz/projects/pkg_def"},
			{desc: "PTest at module root", ModuleDir: "/user/xyz/projects", ModulePath: "github.com/user_xyz/module-abc", PackagePath: "github.com/user_xyz/module-abc", PackageName: "module_abc", Expected: "/user/xyz/projects"},
			{desc: "PXTest at module root", ModuleDir: "/user/xyz/projects", ModulePath: "github.com/user_xyz/module-abc", PackagePath: "github.com/user_xyz/module-abc_test", PackageName: "module_abc_test", Expected: "/user/xyz/projects"},
		} {
			w.It("returns correct dir for "+tC.desc, func(it *gotest.T) {
				actual := gotestgen.DeterminePkgDir(&packages.Package{Module: &packages.Module{Dir: tC.ModuleDir, Path: tC.ModulePath}, PkgPath: tC.PackagePath, Name: tC.PackageName})
				gotest.Equal(it, tC.Expected, actual)
			})
		}
	})
}
