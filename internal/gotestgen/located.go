package gotestgen

import (
	"errors"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestast"
	"golang.org/x/tools/go/packages"
)

// locatedError prints an error behind the place in the source it is about,
// the way the compiler does.
type locatedError struct {
	where string
	err   error
}

func (e *locatedError) Error() string { return e.where + ": " + e.err.Error() }
func (e *locatedError) Unwrap() error { return e.err }

// locate names the position err carries (see gotestast.At). An error without
// one is returned as it is.
func locate(pkg *packages.Package, err error) error {
	if pkg == nil {
		return err
	}
	return locateAt(pkg.Fset, gotestast.PosOf(err), err)
}

func locateAt(fset *token.FileSet, pos token.Pos, err error) error {
	if err == nil || fset == nil || !pos.IsValid() {
		return err
	}
	p := fset.Position(pos)
	if !p.IsValid() {
		return err
	}
	p.Filename = relativeToWorkDir(p.Filename)
	return &locatedError{where: p.String(), err: err}
}

// collectorError joins every error the collector found in a package, each
// behind its position.
func collectorError(pkg *packages.Package, errs []CollectorError) error {
	located := make([]error, 0, len(errs))
	for _, ce := range errs {
		located = append(located, locateAt(pkg.Fset, ce.Pos, ce.Err))
	}
	return errors.Join(located...)
}

func relativeToWorkDir(path string) string {
	wd, err := os.Getwd()
	if err != nil {
		return path
	}
	rel, err := filepath.Rel(wd, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return path
	}
	return rel
}
