package gotestast

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
	"golang.org/x/tools/go/packages"
)

// loadFixtureAST parses src into a type-checked AST and wraps the result in a
// minimal *packages.Package so that DetermineFixture can inspect TypesInfo.
func loadFixtureAST(t *testing.T, src string) (*packages.Package, []*ast.GenDecl) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "fixture.go", src, parser.ParseComments)
	gotest.NoError(t, err)

	conf := types.Config{}
	info := &types.Info{
		Types: make(map[ast.Expr]types.TypeAndValue),
	}
	tpkg, err := conf.Check("testpkg", fset, []*ast.File{f}, info)
	gotest.NoError(t, err)

	pkg := &packages.Package{
		Name:      tpkg.Name(),
		TypesInfo: info,
	}

	var decls []*ast.GenDecl
	for _, d := range f.Decls {
		if gd, ok := d.(*ast.GenDecl); ok && gd.Tok == token.TYPE {
			decls = append(decls, gd)
		}
	}
	return pkg, decls
}

func TestDetermineFixture_PackageFixture(t *testing.T) {
	src := `package testpkg

//go:test fixture
type DBFixture struct {
	Conn string
}
`
	pkg, decls := loadFixtureAST(t, src)
	gotest.Equal(t, 1, len(decls))

	spec, err := DetermineFixture(decls[0], pkg)
	gotest.NoError(t, err)
	gotest.True(t, spec != nil, "expected non-nil FixtureSpec")
	gotest.Equal(t, PackageFixture, spec.Kind)
	gotest.Equal(t, "DBFixture", spec.Identifier())
	gotest.Equal(t, "testpkg", spec.PackageName())
}

func TestDetermineFixture_SharedFixture(t *testing.T) {
	src := `package testpkg

//go:test shared-fixture
type RedisFixture struct {
	Addr string
}
`
	pkg, decls := loadFixtureAST(t, src)
	gotest.Equal(t, 1, len(decls))

	spec, err := DetermineFixture(decls[0], pkg)
	gotest.NoError(t, err)
	gotest.True(t, spec != nil, "expected non-nil FixtureSpec")
	gotest.Equal(t, SharedFixture, spec.Kind)
	gotest.Equal(t, "RedisFixture", spec.Identifier())
	gotest.Equal(t, 0, len(spec.EnvTags))
}

func TestDetermineFixture_EnvTags(t *testing.T) {
	src := "package testpkg\n\n" +
		"//go:test shared-fixture\n" +
		"type EnvFixture struct {\n" +
		"\tHost string `gotest:\"env=DB_HOST\"`\n" +
		"\tPort int    `gotest:\"env=DB_PORT\"`\n" +
		"\tlocal string\n" +
		"\tNoTag string\n" +
		"}\n"

	pkg, decls := loadFixtureAST(t, src)
	gotest.Equal(t, 1, len(decls))

	spec, err := DetermineFixture(decls[0], pkg)
	gotest.NoError(t, err)
	gotest.True(t, spec != nil, "expected non-nil FixtureSpec")
	gotest.Equal(t, SharedFixture, spec.Kind)

	gotest.Equal(t, 2, len(spec.EnvTags))
	gotest.Equal(t, "DB_HOST", spec.EnvTags["Host"])
	gotest.Equal(t, "DB_PORT", spec.EnvTags["Port"])
}

func TestDetermineFixture_NoDirective(t *testing.T) {
	src := `package testpkg

type PlainStruct struct {
	Name string
}
`
	pkg, decls := loadFixtureAST(t, src)
	gotest.Equal(t, 1, len(decls))

	spec, err := DetermineFixture(decls[0], pkg)
	gotest.NoError(t, err)
	gotest.True(t, spec == nil, "expected nil FixtureSpec for struct without directive")
}

func TestDetermineFixture_NonStruct(t *testing.T) {
	src := `package testpkg

//go:test fixture
type BadFixture int
`
	pkg, decls := loadFixtureAST(t, src)
	gotest.Equal(t, 1, len(decls))

	spec, err := DetermineFixture(decls[0], pkg)
	gotest.True(t, err != nil, "expected error for non-struct fixture")
	gotest.True(t, spec == nil, "expected nil FixtureSpec on error")
	gotest.Contains(t, err.Error(), "fixture must be a struct type")
}
