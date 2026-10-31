package gotestgen

import (
	"go/token"
	"sync"

	"golang.org/x/tools/go/packages"
)

// sourceLoader loads the source of single dependencies. The package load
// knows dependencies by export data alone, which carries no method bodies;
// classifying what a fixture's Hydrate assigns needs the declaring package's
// syntax, so that one package is loaded again, as a root. It shares the
// package load's file set, so a dependency's positions resolve there too.
type sourceLoader struct {
	buildFlags []string
	fset       *token.FileSet

	mu     sync.Mutex
	loaded map[string]*packages.Package
}

func newSourceLoader(buildFlags []string, fset *token.FileSet) *sourceLoader {
	return &sourceLoader{buildFlags: buildFlags, fset: fset, loaded: map[string]*packages.Package{}}
}

// load returns the package at pkgPath with syntax and type information, or
// nil when it cannot be loaded cleanly.
func (l *sourceLoader) load(pkgPath string) *packages.Package {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if pkg, ok := l.loaded[pkgPath]; ok {
		return pkg
	}
	cfg := &packages.Config{
		Mode:       packages.NeedName | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo,
		BuildFlags: l.buildFlags,
		Fset:       l.fset,
	}
	var pkg *packages.Package
	if pkgs, err := packages.Load(cfg, pkgPath); err == nil && len(pkgs) == 1 && len(pkgs[0].Errors) == 0 {
		pkg = pkgs[0]
	}
	l.loaded[pkgPath] = pkg
	return pkg
}
