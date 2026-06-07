package gotestgen

import (
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"
)

func DeterminePkgDir(p *packages.Package) string {
	if p.Module == nil {
		if len(p.GoFiles) > 0 {
			return filepath.Dir(p.GoFiles[0])
		}
		return ""
	}
	modDir := p.Module.Dir
	modPath := p.Module.Path
	pkgPath := p.PkgPath
	if isPxTest := strings.HasSuffix(p.Name, "_test"); isPxTest {
		pkgPath = pkgPath[:len(pkgPath)-5] // trim "_test"
	}

	if pkgPath == modPath {
		return modDir
	}

	commonPrefix := len(modPath) + 1
	if len(pkgPath) < commonPrefix {
		return modDir
	}
	path := pkgPath[commonPrefix:]
	return filepath.Join(modDir, path)
}
