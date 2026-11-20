package gotestast

import (
	"go/ast"

	"golang.org/x/tools/go/packages"
)

type ExportRegexpW = regexpW

var ExportValidateContextErrorSig = validateContextErrorSig

// DetermineFixture inspects an AST node for a struct type whose name ends in
// "SharedFixture" (→ SharedFixture kind) or "Fixture" (→ PackageFixture kind).
// Returns nil if the type name doesn't match either suffix, or an error if
// the matching type is not a struct.
func DetermineFixture(n ast.Node, pkg *packages.Package) (*FixtureSpec, error) {
	return DetermineFixtureOf(n, pkg, nil)
}

// ClassifyLocalFields analyzes a Hydrate method's AST to determine which
// exported fixture fields are assigned (directly or via one-level-deep receiver
// method calls). Fields assigned in Hydrate are "local" — they are excluded
// from JSON serialization and reconstructed in the test process.
func ClassifyLocalFields(f *FixtureSpec) map[string]bool {
	if f.HydrateDecl == nil || f.HydrateDecl.Body == nil {
		return nil
	}
	return ClassifyLocalFieldsRaw(f.HydrateDecl, f.Identifier(), f.PackageSyntax(), f.PackageTypesInfo())
}
