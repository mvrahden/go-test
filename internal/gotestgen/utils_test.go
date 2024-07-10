package gotestgen_test

import (
	"testing"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"golang.org/x/tools/go/packages"
)

func Test(t *testing.T) {
	testCases := []struct {
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
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			actual := gotestgen.DeterminePkgDir(&packages.Package{Module: &packages.Module{Dir: tC.ModuleDir, Path: tC.ModulePath}, PkgPath: tC.PackagePath, Name: tC.PackageName})
			if actual != tC.Expected {
				t.Fatalf("not equal. want: %q, got: %q", tC.Expected, actual)
			}
		})
	}
}
