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
		return fmt.Errorf("%s %s (%s) was built with %s, but module %s declares go %s; run the CLI through the tool directive so the module's Go builds it: go get -tool %s/cmd/gotest@latest && go tool gotest ./... (see README, Install)",
			about.Application, about.ResolvedVersion(), exe, builtWith, p.Module.Path, p.Module.GoVersion, about.Repo)
	}
	return nil
}
