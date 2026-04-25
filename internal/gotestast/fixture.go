package gotestast

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"reflect"
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
	directiveFixture       = "//go:test fixture"
	directiveSharedFixture = "//go:test shared-fixture"
)

// FixtureSpec describes a fixture type annotated with a //go:test directive.
type FixtureSpec struct {
	Kind       FixtureKind
	pkg        *packages.Package
	n          ast.Node
	ts         *ast.TypeSpec
	typ        *types.Struct
	BeforeAll  *types.Func
	AfterAll   *types.Func // may be nil
	BeforeEach *types.Func // may be nil
	AfterEach  *types.Func // may be nil
	EnvTags    map[string]string // field name -> env var (shared fixtures only)
}

// Identifier returns the fixture type name.
func (f *FixtureSpec) Identifier() string { return f.ts.Name.Name }

// PackageName returns the package name of the fixture type.
func (f *FixtureSpec) PackageName() string { return f.pkg.Name }

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
