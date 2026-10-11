package goversion_test

import (
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/internal/about"
	"github.com/mvrahden/go-test/internal/goversion"
	"github.com/mvrahden/go-test/pkg/gotest"
	"golang.org/x/tools/go/packages"
)

// GoVersionTestSuite covers the preflight that refuses to run a gotest binary
// built by an older Go than the module declares.
type GoVersionTestSuite struct{}

func (s *GoVersionTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func pkgsDeclaring(goVersion string) []*packages.Package {
	return []*packages.Package{{
		PkgPath: "example.com/app",
		Module:  &packages.Module{Path: "example.com", Main: true, GoVersion: goVersion},
	}}
}

// importingGotest makes each package import gotest from the go-test module
// at version, as the build selected it.
func importingGotest(pkgs []*packages.Package, version string, replaced bool) []*packages.Package {
	mod := &packages.Module{Path: about.Repo, Version: version}
	if replaced {
		mod.Replace = &packages.Module{Path: "../go-test"}
	}
	for _, p := range pkgs {
		p.Imports = map[string]*packages.Package{about.Repo + "/pkg/gotest": {PkgPath: about.Repo + "/pkg/gotest", Module: mod}}
	}
	return pkgs
}

func (s *GoVersionTestSuite) TestCheck(t *gotest.T) {
	t.When("the binary's Go is older than the module's go directive", func(w *gotest.T) {
		w.It("fails naming both versions and the rebuild command", func(it *gotest.T) {
			err := goversion.ExportCheck(pkgsDeclaring("1.27.1"), "go1.26.3", "v1.32.0", goversion.ExportSelected)
			gotest.ErrorContains(it, err, "go1.26.3")
			gotest.ErrorContains(it, err, "go 1.27.1")
			gotest.ErrorContains(it, err, "go tool gotest")
		})
		w.It("compares language versions, so a directive without patch still counts", func(it *gotest.T) {
			gotest.Error(it, goversion.ExportCheck(pkgsDeclaring("1.27"), "go1.26.5", "v1.32.0", goversion.ExportSelected))
		})
	})

	t.When("the drift message names the gotest version to pin", func(w *gotest.T) {
		w.It("names the version the loaded packages import", func(it *gotest.T) {
			err := goversion.ExportCheck(importingGotest(pkgsDeclaring("1.27.1"), "v1.31.0", false), "go1.26.3", "", goversion.ExportUnknown)
			gotest.ErrorContains(it, err, "go get -tool "+about.Repo+"/cmd/gotest@v1.31.0 && go tool gotest")
		})
		w.It("takes it from a sibling workspace module when the tripping one imports none", func(it *gotest.T) {
			sibling := importingGotest([]*packages.Package{{
				PkgPath: "example.com/m1",
				Module:  &packages.Module{Path: "example.com/m1", GoVersion: "1.26"},
			}}, "v1.32.0", false)
			err := goversion.ExportCheck(append(pkgsDeclaring("1.27.1"), sibling...), "go1.26.3", "", goversion.ExportUnknown)
			gotest.ErrorContains(it, err, "/cmd/gotest@v1.32.0 ")
		})
		w.It("keeps the pseudo-version a replace directive resolves", func(it *gotest.T) {
			pkgs := importingGotest(pkgsDeclaring("1.27.1"), "v0.0.0-00010101000000-000000000000", true)
			err := goversion.ExportCheck(pkgs, "go1.26.3", "", goversion.ExportUnknown)
			gotest.ErrorContains(it, err, "/cmd/gotest@v0.0.0-00010101000000-000000000000 ")
		})
		w.It("names the version the module requires when no package imports gotest", func(it *gotest.T) {
			err := goversion.ExportCheck(pkgsDeclaring("1.27.1"), "go1.26.3", "v1.32.0", goversion.ExportSelected)
			gotest.ErrorContains(it, err, "go get -tool "+about.Repo+"/cmd/gotest@v1.32.0 && go tool gotest")
		})
		w.It("names no version for a workspace module, which has no pin to move", func(it *gotest.T) {
			err := goversion.ExportCheck(pkgsDeclaring("1.27.1"), "go1.26.3", "", goversion.ExportSelected)
			gotest.ErrorContains(it, err, "go get -tool "+about.Repo+"/cmd/gotest && go tool gotest")
		})
		w.It("asks to declare a fitting release when the module does not require gotest", func(it *gotest.T) {
			err := goversion.ExportCheck(pkgsDeclaring("1.27.1"), "go1.26.3", "", goversion.ExportNotRequired)
			gotest.ErrorContains(it, err, "module example.com does not require go-test: declare the tool at a release whose go line the module takes")
			gotest.NotContains(it, err.Error(), "@latest")
		})
		w.It("asks to declare a fitting release, claiming nothing, when the go command cannot tell", func(it *gotest.T) {
			err := goversion.ExportCheck(pkgsDeclaring("1.27.1"), "go1.26.3", "", goversion.ExportUnknown)
			gotest.ErrorContains(it, err, "declare the tool at a release whose go line the module takes")
			gotest.NotContains(it, err.Error(), "does not require")
			gotest.NotContains(it, err.Error(), "@latest")
		})
	})

	t.When("the go command reports the module's go-test selection", func(w *gotest.T) {
		w.It("names the version go.mod requires, resolved through a replace", func(it *gotest.T) {
			dir := it.TempDir()
			writeFile(it, filepath.Join(dir, "gt", "go.mod"), "module "+about.Repo+"\n\ngo 1.24\n")
			writeFile(it, filepath.Join(dir, "app", "go.mod"), "module example.com/app\n\ngo 1.24\n\nrequire "+about.Repo+" v1.32.0\n\nreplace "+about.Repo+" => ../gt\n")
			v, sel := goversion.ExportSelectedVersion(filepath.Join(dir, "app"))
			gotest.Equal(it, "v1.32.0", v)
			gotest.Equal(it, goversion.ExportSelected, sel)
		})
		w.It("says so when the module does not depend on go-test", func(it *gotest.T) {
			dir := it.TempDir()
			writeFile(it, filepath.Join(dir, "go.mod"), "module example.com/app\n\ngo 1.24\n")
			_, sel := goversion.ExportSelectedVersion(dir)
			gotest.Equal(it, goversion.ExportNotRequired, sel)
		})
	})

	t.When("the binary's Go can process the module", func(w *gotest.T) {
		w.It("accepts an equal language version regardless of patch level", func(it *gotest.T) {
			gotest.NoError(it, goversion.ExportCheck(pkgsDeclaring("1.27.1"), "go1.27.0", "", goversion.ExportUnknown))
		})
		w.It("accepts a release candidate of the same language version", func(it *gotest.T) {
			gotest.NoError(it, goversion.ExportCheck(pkgsDeclaring("1.27.1"), "go1.27rc1", "", goversion.ExportUnknown))
		})
		w.It("accepts a newer Go", func(it *gotest.T) {
			gotest.NoError(it, goversion.ExportCheck(pkgsDeclaring("1.26"), "go1.27.0", "", goversion.ExportUnknown))
		})
	})

	t.When("a version cannot be compared", func(w *gotest.T) {
		w.It("skips development toolchains", func(it *gotest.T) {
			gotest.NoError(it, goversion.ExportCheck(pkgsDeclaring("1.27.1"), "devel go1.28-abcdef Mon Jan 1", "", goversion.ExportUnknown))
		})
		w.It("skips packages without module information", func(it *gotest.T) {
			gotest.NoError(it, goversion.ExportCheck([]*packages.Package{{PkgPath: "fmt"}}, "go1.26.3", "", goversion.ExportUnknown))
		})
		w.It("skips a module without a go directive", func(it *gotest.T) {
			gotest.NoError(it, goversion.ExportCheck(pkgsDeclaring(""), "go1.26.3", "", goversion.ExportUnknown))
		})
	})
}

func writeFile(t *gotest.T, path, content string) {
	gotest.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	gotest.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}
