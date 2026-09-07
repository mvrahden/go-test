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
	return gotest.SuiteConfig{Parallel: true}
}

func pkgsDeclaring(goVersion string) []*packages.Package {
	return []*packages.Package{{
		PkgPath: "example.com/app",
		Module:  &packages.Module{Path: "example.com", Main: true, GoVersion: goVersion},
	}}
}

func (s *GoVersionTestSuite) TestCheck(t *gotest.T) {
	t.When("the binary's Go is older than the module's go directive", func(w *gotest.T) {
		w.It("fails naming both versions and the rebuild command", func(it *gotest.T) {
			err := goversion.ExportCheck(pkgsDeclaring("1.27.1"), "go1.26.3")
			gotest.ErrorContains(it, err, "go1.26.3")
			gotest.ErrorContains(it, err, "go 1.27.1")
			gotest.ErrorContains(it, err, "go get -tool "+about.Repo+"/cmd/gotest@latest")
			gotest.ErrorContains(it, err, "go tool gotest")
		})
		w.It("compares language versions, so a directive without patch still counts", func(it *gotest.T) {
			gotest.Error(it, goversion.ExportCheck(pkgsDeclaring("1.27"), "go1.26.5"))
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
