package gotestgen

import (
	"fmt"

	"github.com/mvrahden/go-test/internal/about"
	"golang.org/x/mod/semver"
	"golang.org/x/tools/go/packages"
)

// MinRuntimeVersion is the oldest gotest runtime generated code compiles
// against. It equals the extension's MIN_CLI_VERSION (a test guards the pair);
// bump both when the renderer uses a newer runtime symbol.
const MinRuntimeVersion = "v1.27.0"

// CheckRuntimeVersion refuses to generate against a runtime older than
// MinRuntimeVersion. Replaced and main modules have no version to check.
func CheckRuntimeVersion(loadResults []*LoadResult) error {
	for _, lr := range loadResults {
		for _, p := range []*packages.Package{lr.Ptest, lr.Pxtest} {
			if p == nil {
				continue
			}
			mod := gotestModule(p)
			if mod == nil || mod.Main || mod.Replace != nil || mod.Version == "" {
				continue
			}
			if semver.Compare(mod.Version, MinRuntimeVersion) < 0 {
				return fmt.Errorf("%s %s requires %s %s or newer, but go.mod resolves %s; run: go get -tool %s/cmd/gotest@latest, then use go tool gotest (see README, Install)",
					about.Application, about.ResolvedVersion(), about.Repo, MinRuntimeVersion, mod.Version, about.Repo)
			}
			return nil
		}
	}
	return nil
}

func gotestModule(p *packages.Package) *packages.Module {
	for _, imp := range p.Imports {
		if imp.Module != nil && imp.Module.Path == about.Repo {
			return imp.Module
		}
	}
	return nil
}
