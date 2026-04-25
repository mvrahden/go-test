package gotestast

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"reflect"
	"strings"

	"github.com/mvrahden/go-test/about"
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
	directiveFixture       = "//go:test fixture"
	directiveSharedFixture = "//go:test shared-fixture"
)

// FixtureSpec describes a fixture type annotated with a //go:test directive.
type FixtureSpec struct {
	Kind          FixtureKind
	pkg           *packages.Package
	n             ast.Node
	ts            *ast.TypeSpec
	typ           *types.Struct
	BeforeAll     *types.Func
	AfterAll      *types.Func // may be nil
	BeforeEach    *types.Func // may be nil
	AfterEach     *types.Func // may be nil
	EnvTags       map[string]string // field name -> env var (shared fixtures only)
	ParentFixture *FixtureSpec      // pointer to parent fixture (via embedding), may be nil
}

// Identifier returns the fixture type name.
func (f *FixtureSpec) Identifier() string { return f.ts.Name.Name }

// PackageName returns the package name of the fixture type.
func (f *FixtureSpec) PackageName() string { return f.pkg.Name }

// PackagePath returns the import path of the package that defines this fixture.
func (f *FixtureSpec) PackagePath() string { return f.pkg.PkgPath }

// StructType returns the underlying *types.Struct for field inspection.
func (f *FixtureSpec) StructType() *types.Struct { return f.typ }

// DetermineFixture inspects an AST node for a //go:test fixture or //go:test shared-fixture
// directive. It returns the FixtureSpec if found, nil if no directive is present, or an error
// if the directive is applied to a non-struct type.
func DetermineFixture(n ast.Node, pkg *packages.Package) (*FixtureSpec, error) {
	decl, ok := n.(*ast.GenDecl)
	if !ok || decl.Tok != token.TYPE || len(decl.Specs) != 1 {
		return nil, nil
	}

	// Check doc comment for directive
	kind := FixtureKind(-1)
	if decl.Doc != nil {
		for _, c := range decl.Doc.List {
			text := strings.TrimSpace(c.Text)
			switch text {
			case directiveFixture:
				kind = PackageFixture
			case directiveSharedFixture:
				kind = SharedFixture
			}
		}
	}
	if kind < 0 {
		return nil, nil
	}

	ts, ok := decl.Specs[0].(*ast.TypeSpec)
	if !ok {
		return nil, nil
	}

	rawType := pkg.TypesInfo.TypeOf(ts.Type)
	typ, ok := rawType.(*types.Struct)
	if !ok {
		return nil, fmt.Errorf("%s: fixture must be a struct type", ts.Name.Name)
	}

	spec := &FixtureSpec{
		Kind: kind, pkg: pkg, n: n, ts: ts, typ: typ,
	}

	// For shared fixtures, parse gotest:"env=..." struct tags
	if kind == SharedFixture {
		spec.EnvTags = parseEnvTags(typ)
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
	// Only care about lifecycle methods
	if name != "BeforeAll" && name != "AfterAll" && name != "BeforeEach" && name != "AfterEach" {
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

	switch f.Kind {
	case PackageFixture:
		// Package fixture lifecycle methods: (t *gotest.T), no results
		if name == "BeforeEach" || name == "AfterEach" || name == "BeforeAll" || name == "AfterAll" {
			if sig.Params().Len() != 1 || sig.Results().Len() != 0 {
				return m.Pos(), fmt.Errorf("unsupported signature for %q: expected (t *gotest.T)", methodID)
			}
			pT := sig.Params().At(0).Type().String()
			if !strings.HasPrefix(pT, "*"+about.Repo) || !strings.HasSuffix(pT, "/gotest.T") {
				return m.Pos(), fmt.Errorf("unsupported param type for signature of %q: expected *gotest.T", methodID)
			}
		}
	case SharedFixture:
		// Shared fixture: BeforeAll/AfterAll must be () error, no BeforeEach/AfterEach
		if name == "BeforeEach" || name == "AfterEach" {
			return m.Pos(), fmt.Errorf("shared fixture %q must not have %s method", f.ts.Name.Name, name)
		}
		if sig.Params().Len() != 0 || sig.Results().Len() != 1 {
			return m.Pos(), fmt.Errorf("unsupported signature for %q: expected () error", methodID)
		}
		resType := sig.Results().At(0).Type().String()
		if resType != "error" {
			return m.Pos(), fmt.Errorf("unsupported return type for %q: expected error, got %s", methodID, resType)
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

func parseEnvTags(typ *types.Struct) map[string]string {
	tags := make(map[string]string)
	for i := 0; i < typ.NumFields(); i++ {
		f := typ.Field(i)
		rawTag := typ.Tag(i)
		if !f.Exported() || rawTag == "" {
			continue
		}
		st := reflect.StructTag(rawTag)
		val, ok := st.Lookup("gotest")
		if !ok {
			continue
		}
		if strings.HasPrefix(val, "env=") {
			tags[f.Name()] = val[4:]
		}
	}
	return tags
}
