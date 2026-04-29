package gotestast

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/packages"
)

// FixtureKind distinguishes package-scoped fixtures from shared (cross-package) fixtures.
type FixtureKind int

const (
	FixtureKindUnknown FixtureKind = iota
	PackageFixture
	SharedFixture
)

const (
	suffixFixture       = "Fixture"
	suffixSharedFixture = "SharedFixture"
)

// FixtureSpec describes a fixture type identified by naming convention.
type FixtureSpec struct {
	Kind          FixtureKind
	pkg           *packages.Package
	n             ast.Node
	ts            *ast.TypeSpec
	typ           *types.Struct
	Config        *types.Func      // FixtureConfig()/SharedFixtureConfig() method, may be nil
	BeforeAll     *types.Func
	AfterAll      *types.Func      // may be nil
	BeforeEach    *types.Func      // may be nil
	AfterEach     *types.Func      // may be nil
	Hydrate       *types.Func   // shared fixtures only, may be nil
	Dehydrate     *types.Func   // shared fixtures only, may be nil
	HydrateDecl   *ast.FuncDecl // AST for Hydrate body analysis, may be nil
	ParentFixture *FixtureSpec  // pointer to parent fixture (via embedding), may be nil
}

// Identifier returns the fixture type name.
func (f *FixtureSpec) Identifier() string { return f.ts.Name.Name }

// PackageName returns the package name of the fixture type.
func (f *FixtureSpec) PackageName() string { return f.pkg.Name }

// PackagePath returns the import path of the package that defines this fixture.
func (f *FixtureSpec) PackagePath() string { return f.pkg.PkgPath }

// StructType returns the underlying *types.Struct for field inspection.
func (f *FixtureSpec) StructType() *types.Struct { return f.typ }

// PackageSyntax returns the parsed AST files for the fixture's package.
func (f *FixtureSpec) PackageSyntax() []*ast.File { return f.pkg.Syntax }

// PackageTypesInfo returns the type-checking results for the fixture's package.
func (f *FixtureSpec) PackageTypesInfo() *types.Info { return f.pkg.TypesInfo }

// ExportedFieldNames returns the names of all exported fields on the fixture struct.
func (f *FixtureSpec) ExportedFieldNames() []string {
	if f.typ == nil {
		return nil
	}
	var names []string
	for i := 0; i < f.typ.NumFields(); i++ {
		field := f.typ.Field(i)
		if field.Exported() && !field.Anonymous() {
			names = append(names, field.Name())
		}
	}
	return names
}

// DetermineFixture inspects an AST node for a struct type whose name ends in
// "SharedFixture" (→ SharedFixture kind) or "Fixture" (→ PackageFixture kind).
// Returns nil if the type name doesn't match either suffix, or an error if
// the matching type is not a struct.
func DetermineFixture(n ast.Node, pkg *packages.Package) (*FixtureSpec, error) {
	decl, ok := n.(*ast.GenDecl)
	if !ok || decl.Tok != token.TYPE || len(decl.Specs) != 1 {
		return nil, nil
	}

	ts, ok := decl.Specs[0].(*ast.TypeSpec)
	if !ok {
		return nil, nil
	}

	name := ts.Name.Name
	// Check SharedFixture first (it also ends in "Fixture")
	kind := FixtureKind(-1)
	switch {
	case strings.HasSuffix(name, suffixSharedFixture):
		kind = SharedFixture
	case strings.HasSuffix(name, suffixFixture):
		kind = PackageFixture
	}
	if kind < 0 {
		return nil, nil
	}

	// Avoid matching *TestSuite types that happen to embed a fixture
	// (e.g. "MyTestSuiteFixture" shouldn't match as a suite)
	// — but any standalone struct ending in Fixture is a fixture.

	rawType := pkg.TypesInfo.TypeOf(ts.Type)
	typ, ok := rawType.(*types.Struct)
	if !ok {
		return nil, fmt.Errorf("%s: fixture must be a struct type", name)
	}

	spec := &FixtureSpec{
		Kind: kind, pkg: pkg, n: n, ts: ts, typ: typ,
	}

	return spec, nil
}

// DetermineFixtureHarness inspects a FuncDecl AST node to see if it is a
// lifecycle method (BeforeAll, AfterAll, BeforeEach, AfterEach) on the given
// fixture spec. It validates the method signature according to the fixture kind
// and populates the corresponding field on the FixtureSpec.
func DetermineFixtureHarness(n ast.Node, pkg *packages.Package, f *FixtureSpec) (token.Pos, error) {
	decl, ok := n.(*ast.FuncDecl)
	if !ok {
		return -1, nil
	}
	if !decl.Name.IsExported() {
		return -1, nil
	}
	m, ok := pkg.TypesInfo.ObjectOf(decl.Name).(*types.Func)
	if !ok {
		return -1, nil
	}

	name := m.Name()
	// Only care about lifecycle methods, config, and hydrate/dehydrate
	switch name {
	case "BeforeAll", "AfterAll", "BeforeEach", "AfterEach",
		"FixtureConfig", "SharedFixtureConfig",
		"Hydrate", "Dehydrate":
	default:
		return -1, nil
	}

	sig, ok := pkg.TypesInfo.TypeOf(decl.Name).(*types.Signature)
	if !ok {
		return -1, nil
	}
	recv := sig.Recv()
	if recv == nil {
		return -1, nil
	}

	// Must be a pointer receiver
	recvPtr, ok := recv.Type().(*types.Pointer)
	if !ok {
		return -1, nil
	}
	recvType, ok := recvPtr.Elem().(*types.Named)
	if !ok || recvType == nil {
		return -1, nil
	}

	// Must match the fixture type name
	if recvType.Obj().Name() != f.ts.Name.Name {
		return -1, nil
	}

	methodID := f.ts.Name.Name + "." + name

	// Config marker methods — dispatch by kind
	if name == "FixtureConfig" {
		if f.Kind == SharedFixture {
			return m.Pos(), fmt.Errorf("shared fixture %q should use SharedFixtureConfig(), not FixtureConfig()", f.ts.Name.Name)
		}
		if sig.Params().Len() != 0 || sig.Results().Len() != 1 {
			return m.Pos(), fmt.Errorf("unsupported signature for %q: expected () gotest.FixtureConfig", methodID)
		}
		resType := sig.Results().At(0).Type().String()
		if !strings.HasSuffix(resType, "/gotest.FixtureConfig") {
			return m.Pos(), fmt.Errorf("unsupported return type for %q: expected gotest.FixtureConfig, got %s", methodID, resType)
		}
		f.Config = m
		return -1, nil
	}
	if name == "SharedFixtureConfig" {
		if f.Kind == PackageFixture {
			return m.Pos(), fmt.Errorf("package fixture %q should use FixtureConfig(), not SharedFixtureConfig()", f.ts.Name.Name)
		}
		if sig.Params().Len() != 0 || sig.Results().Len() != 1 {
			return m.Pos(), fmt.Errorf("unsupported signature for %q: expected () gotest.FixtureConfig", methodID)
		}
		resType := sig.Results().At(0).Type().String()
		if !strings.HasSuffix(resType, "/gotest.FixtureConfig") {
			return m.Pos(), fmt.Errorf("unsupported return type for %q: expected gotest.FixtureConfig, got %s", methodID, resType)
		}
		f.Config = m
		return -1, nil
	}

	// Hydrate/Dehydrate — shared fixtures only
	if name == "Hydrate" || name == "Dehydrate" {
		if f.Kind != SharedFixture {
			return m.Pos(), fmt.Errorf("package fixture %q must not have %s method; Hydrate/Dehydrate are for shared fixtures only", f.ts.Name.Name, name)
		}
		if err := validateContextErrorSig(sig, methodID); err != nil {
			return m.Pos(), err
		}
		if name == "Hydrate" {
			f.Hydrate = m
			f.HydrateDecl = decl
		} else {
			f.Dehydrate = m
		}
		return -1, nil
	}

	// Lifecycle methods — validate per kind
	switch f.Kind {
	case PackageFixture:
		if err := validateContextErrorSig(sig, methodID); err != nil {
			return m.Pos(), err
		}
	case SharedFixture:
		if name == "BeforeEach" || name == "AfterEach" {
			return m.Pos(), fmt.Errorf("shared fixture %q must not have %s method", f.ts.Name.Name, name)
		}
		if err := validateContextErrorSig(sig, methodID); err != nil {
			return m.Pos(), err
		}
	default:
		return m.Pos(), fmt.Errorf("unknown fixture kind for %q", methodID)
	}

	switch name {
	case "BeforeAll":
		f.BeforeAll = m
	case "AfterAll":
		f.AfterAll = m
	case "BeforeEach":
		f.BeforeEach = m
	case "AfterEach":
		f.AfterEach = m
	}

	return -1, nil
}

// NewFixtureSpecForTest creates a minimal FixtureSpec for use in unit tests.
// It sets only the Kind and ts fields (so Identifier() works).
func NewFixtureSpecForTest(name string, kind FixtureKind) *FixtureSpec {
	return &FixtureSpec{
		Kind: kind,
		pkg:  &packages.Package{},
		ts:   &ast.TypeSpec{Name: ast.NewIdent(name)},
	}
}

// NewFixtureSpecForTestWithPkg creates a minimal FixtureSpec for use in unit tests,
// including a package path so that PackagePath() works.
func NewFixtureSpecForTestWithPkg(name string, kind FixtureKind, pkgPath string) *FixtureSpec {
	return &FixtureSpec{
		Kind: kind,
		pkg:  &packages.Package{PkgPath: pkgPath},
		ts:   &ast.TypeSpec{Name: ast.NewIdent(name)},
	}
}

func validateContextErrorSig(sig *types.Signature, methodID string) error {
	if sig.Params().Len() != 1 || sig.Results().Len() != 1 {
		return fmt.Errorf("unsupported signature for %q: expected (context.Context) error", methodID)
	}
	paramType := sig.Params().At(0).Type().String()
	if paramType != "context.Context" {
		return fmt.Errorf("unsupported param type for %q: expected context.Context, got %s", methodID, paramType)
	}
	resType := sig.Results().At(0).Type().String()
	if resType != "error" {
		return fmt.Errorf("unsupported return type for %q: expected error, got %s", methodID, resType)
	}
	return nil
}

