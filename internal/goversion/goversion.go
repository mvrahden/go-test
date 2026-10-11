// Package goversion refuses to run a gotest binary built by an older Go than
// the module declares, naming the binary and the fix instead of the type
// checker's "application" message. Only a stale `go install` binary trips it.
package goversion

import (
	"fmt"
	"go/version"
	"os"
	"runtime"

	"github.com/mvrahden/go-test/internal/about"
	"golang.org/x/mod/semver"
	"golang.org/x/tools/go/packages"
)

// Check compares this binary's Go with each package's module go directive.
// Packages must be loaded with packages.NeedModule.
func Check(pkgs []*packages.Package) error {
	return check(pkgs, runtime.Version())
}

func check(pkgs []*packages.Package, builtWith string) error {
	built := version.Lang(builtWith)
	if built == "" {
		return nil
	}
	for _, p := range pkgs {
		if p.Module == nil || p.Module.GoVersion == "" {
			continue
		}
		declared := version.Lang("go" + p.Module.GoVersion)
		if declared == "" || version.Compare(built, declared) >= 0 {
			continue
		}
		exe, _ := os.Executable()
		return fmt.Errorf("%s %s (%s) was built with %s, but module %s declares go %s; run the CLI through the tool directive so the module's Go builds it: go get -tool %s/cmd/gotest@%s && go tool gotest ./... (see README, Install)",
			about.Application, about.ResolvedVersion(), exe, builtWith, p.Module.Path, p.Module.GoVersion, about.Repo, pinnedVersion(pkgs, about.ResolvedVersion()))
	}
	return nil
}

// pinnedVersion is the gotest version the build selects, so declaring the
// tool moves no pin (@latest could raise the go line): the go-test module
// the loaded packages import, whichever go.mod, go.work or replace chose it.
// Without one it is self, this binary's version, if a release, else latest.
func pinnedVersion(pkgs []*packages.Package, self string) string {
	for _, p := range pkgs {
		for _, imp := range p.Imports {
			if m := imp.Module; m != nil && m.Path == about.Repo && m.Version != "" {
				return m.Version
			}
		}
	}
	if semver.IsValid(self) {
		return self
	}
	return "latest"
}
