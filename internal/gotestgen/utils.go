package gotestgen

import (
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"
)

func DeterminePkgDir(p *packages.Package) string {
	modDir := p.Module.Dir
	modPath := p.Module.Path
	pkgPath := p.PkgPath
	if isPxTest := strings.HasSuffix(p.Name, "_test"); isPxTest {
		pkgPath = pkgPath[:len(pkgPath)-5] // trim "_test"
	}

	commonPrefix := len(modPath) + 1
	path := pkgPath[commonPrefix:]
	return filepath.Join(modDir, path)
}
