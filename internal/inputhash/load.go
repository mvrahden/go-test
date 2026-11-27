package inputhash

import (
	"context"
	"go/token"

	"github.com/mvrahden/go-test/internal/genstub"
	"github.com/mvrahden/go-test/internal/pkgload"
)

// Current hashes the packages of the tree in dir ("" the current
// directory) under the build flags a run recorded, and the settings the go
// command has now. A package the tree no longer holds is left out.
// loadFlags are recorded flags a load takes: without the compile-only ones.
func Current(ctx context.Context, dir string, pkgs, recorded, loadFlags []string) (Hashes, error) {
	if len(pkgs) == 0 {
		return Hashes{}, nil
	}
	settings, err := ReadSettings(ctx, dir, recorded)
	if err != nil {
		return nil, err
	}
	flags, unstub, err := genstub.LoadFlags(dir, pkgs, loadFlags)
	if err != nil {
		return nil, err
	}
	defer unstub()
	roots, err := pkgload.Graph(&pkgload.Config{Context: ctx, Dir: dir, BuildFlags: flags, Tests: true, Fset: token.NewFileSet()}, pkgs...)
	if err != nil {
		return nil, err
	}
	kept := roots[:0]
	for _, p := range roots {
		if len(p.GoFiles)+len(p.OtherFiles) > 0 {
			kept = append(kept, p)
		}
	}
	return Packages(kept, settings), nil
}
