package goversion_test

import (
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
			err := goversion.ExportCheck(pkgsDeclaring("1.27.1"), "go1.26.3")
			gotest.ErrorContains(it, err, "go1.26.3")
			gotest.ErrorContains(it, err, "go 1.27.1")
			gotest.ErrorContains(it, err, "go tool gotest")
		})
		w.It("names the gotest version the build selects, never @latest", func(it *gotest.T) {
			err := goversion.ExportCheck(importingGotest(pkgsDeclaring("1.27.1"), "v1.31.0", false), "go1.26.3")
			gotest.ErrorContains(it, err, "go get -tool "+about.Repo+"/cmd/gotest@v1.31.0 && go tool gotest")
		})
		w.It("compares language versions, so a directive without patch still counts", func(it *gotest.T) {
			gotest.Error(it, goversion.ExportCheck(pkgsDeclaring("1.27"), "go1.26.5"))
		})
	})

	t.When("the drift message names the gotest version to pin", func(w *gotest.T) {
		w.It("takes it from a sibling workspace module when the tripping one requires none", func(it *gotest.T) {
			sibling := importingGotest([]*packages.Package{{
				PkgPath: "example.com/m1",
				Module:  &packages.Module{Path: "example.com/m1", GoVersion: "1.26"},
			}}, "v1.32.0", false)
			pkgs := append(pkgsDeclaring("1.27.1"), sibling...)
			gotest.Equal(it, "v1.32.0", goversion.ExportPinnedVersion(pkgs, "v1.33.0"))
		})
		w.It("keeps the pseudo-version a replace directive resolves", func(it *gotest.T) {
			pkgs := importingGotest(pkgsDeclaring("1.27.1"), "v0.0.0-00010101000000-000000000000", true)
			gotest.Equal(it, "v0.0.0-00010101000000-000000000000", goversion.ExportPinnedVersion(pkgs, "v1.33.0"))
		})
		w.It("falls back to this binary's release when no package imports gotest", func(it *gotest.T) {
			gotest.Equal(it, "v1.33.0", goversion.ExportPinnedVersion(pkgsDeclaring("1.27.1"), "v1.33.0"))
		})
		w.It("falls back to latest for a binary without a release version", func(it *gotest.T) {
			gotest.Equal(it, "latest", goversion.ExportPinnedVersion(pkgsDeclaring("1.27.1"), "dev (source checkout)"))
		})
	})

	t.When("the binary's Go can process the module", func(w *gotest.T) {
		w.It("accepts an equal language version regardless of patch level", func(it *gotest.T) {
			gotest.NoError(it, goversion.ExportCheck(pkgsDeclaring("1.27.1"), "go1.27.0"))
		})
		w.It("accepts a release candidate of the same language version", func(it *gotest.T) {
			gotest.NoError(it, goversion.ExportCheck(pkgsDeclaring("1.27.1"), "go1.27rc1"))
		})
		w.It("accepts a newer Go", func(it *gotest.T) {
			gotest.NoError(it, goversion.ExportCheck(pkgsDeclaring("1.26"), "go1.27.0"))
		})
	})

	t.When("a version cannot be compared", func(w *gotest.T) {
		w.It("skips development toolchains", func(it *gotest.T) {
			gotest.NoError(it, goversion.ExportCheck(pkgsDeclaring("1.27.1"), "devel go1.28-abcdef Mon Jan 1"))
		})
		w.It("skips packages without module information", func(it *gotest.T) {
			gotest.NoError(it, goversion.ExportCheck([]*packages.Package{{PkgPath: "fmt"}}, "go1.26.3"))
		})
		w.It("skips a module without a go directive", func(it *gotest.T) {
			gotest.NoError(it, goversion.ExportCheck(pkgsDeclaring(""), "go1.26.3"))
		})
	})
}
