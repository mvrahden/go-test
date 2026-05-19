package gotestast //nolint:stdlib-test

import (
	"go/ast"
	"go/importer"
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

type RedisSharedFixture struct {
	Addr string
}
`
	pkg, decls := loadFixtureAST(t, src)
	gotest.Equal(t, 1, len(decls))

	spec, err := DetermineFixture(decls[0], pkg)
	gotest.NoError(t, err)
	gotest.True(t, spec != nil, "expected non-nil FixtureSpec")
	gotest.Equal(t, SharedFixture, spec.Kind)
	gotest.Equal(t, "RedisSharedFixture", spec.Identifier())
}

func TestDetermineFixture_NoSuffix(t *testing.T) {
	src := `package testpkg

type PlainStruct struct {
	Name string
}
`
	pkg, decls := loadFixtureAST(t, src)
	gotest.Equal(t, 1, len(decls))

	spec, err := DetermineFixture(decls[0], pkg)
	gotest.NoError(t, err)
	gotest.True(t, spec == nil, "expected nil FixtureSpec for struct without Fixture suffix")
}

func TestDetermineFixture_NonStruct(t *testing.T) {
	src := `package testpkg

type BadFixture int
`
	pkg, decls := loadFixtureAST(t, src)
	gotest.Equal(t, 1, len(decls))

	spec, err := DetermineFixture(decls[0], pkg)
	gotest.True(t, err != nil, "expected error for non-struct fixture")
	gotest.True(t, spec == nil, "expected nil FixtureSpec on error")
	gotest.Contains(t, err.Error(), "fixture must be a struct type")
}

type fixtureWithMethods struct {
	pkg       *packages.Package
	genDecls  []*ast.GenDecl
	funcDecls []*ast.FuncDecl
}

func loadFixtureWithMethods(t *testing.T, src string) fixtureWithMethods {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "fixture.go", src, parser.ParseComments)
	gotest.NoError(t, err)

	conf := types.Config{Importer: importer.Default()}
	info := &types.Info{
		Types: make(map[ast.Expr]types.TypeAndValue),
		Defs:  make(map[*ast.Ident]types.Object),
		Uses:  make(map[*ast.Ident]types.Object),
	}
	tpkg, err := conf.Check("testpkg", fset, []*ast.File{f}, info)
	gotest.NoError(t, err)

	pkg := &packages.Package{
		Name:      tpkg.Name(),
		PkgPath:   tpkg.Path(),
		TypesInfo: info,
		Syntax:    []*ast.File{f},
	}

	var genDecls []*ast.GenDecl
	var funcDecls []*ast.FuncDecl
	for _, d := range f.Decls {
		switch dd := d.(type) {
		case *ast.GenDecl:
			if dd.Tok == token.TYPE {
				genDecls = append(genDecls, dd)
			}
		case *ast.FuncDecl:
			funcDecls = append(funcDecls, dd)
		}
	}
	return fixtureWithMethods{pkg: pkg, genDecls: genDecls, funcDecls: funcDecls}
}

func TestDetermineFixtureHarness_SharedFixture_Lifecycle(t *testing.T) {
	src := `package testpkg

import "context"

type PGSharedFixture struct {
	ConnStr string
}

func (f *PGSharedFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *PGSharedFixture) AfterAll(ctx context.Context) error  { return nil }
`
	fm := loadFixtureWithMethods(t, src)
	gotest.Equal(t, 1, len(fm.genDecls))
	gotest.Equal(t, 2, len(fm.funcDecls))

	spec, err := DetermineFixture(fm.genDecls[0], fm.pkg)
	gotest.NoError(t, err)
	gotest.True(t, spec != nil)
	gotest.Equal(t, SharedFixture, spec.Kind)

	for _, fd := range fm.funcDecls {
		_, err := DetermineFixtureHarness(fd, fm.pkg, spec)
		gotest.NoError(t, err)
	}

	gotest.True(t, spec.BeforeAll != nil, "BeforeAll should be set")
	gotest.True(t, spec.AfterAll != nil, "AfterAll should be set")
}

func TestDetermineFixtureHarness_SharedFixture_BeforeEach_Rejected(t *testing.T) {
	src := `package testpkg

import "context"

type BadSharedFixture struct {
	Addr string
}

func (f *BadSharedFixture) BeforeAll(ctx context.Context) error  { return nil }
func (f *BadSharedFixture) BeforeEach(ctx context.Context) error { return nil }
`
	fm := loadFixtureWithMethods(t, src)
	spec, err := DetermineFixture(fm.genDecls[0], fm.pkg)
	gotest.NoError(t, err)

	for _, fd := range fm.funcDecls {
		pos, err := DetermineFixtureHarness(fd, fm.pkg, spec)
		if fd.Name.Name == "BeforeEach" {
			gotest.True(t, err != nil, "BeforeEach should be rejected on shared fixture")
			gotest.Contains(t, err.Error(), "must not have BeforeEach method")
			gotest.True(t, pos > 0, "error position should be set")
		}
	}
}

func TestDetermineFixtureHarness_SharedFixture_AfterEach_Rejected(t *testing.T) {
	src := `package testpkg

import "context"

type BadSharedFixture struct {
	Addr string
}

func (f *BadSharedFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *BadSharedFixture) AfterEach(ctx context.Context) error { return nil }
`
	fm := loadFixtureWithMethods(t, src)
	spec, err := DetermineFixture(fm.genDecls[0], fm.pkg)
	gotest.NoError(t, err)

	for _, fd := range fm.funcDecls {
		pos, err := DetermineFixtureHarness(fd, fm.pkg, spec)
		if fd.Name.Name == "AfterEach" {
			gotest.True(t, err != nil, "AfterEach should be rejected on shared fixture")
			gotest.Contains(t, err.Error(), "must not have AfterEach method")
			gotest.True(t, pos > 0, "error position should be set")
		}
	}
}

func TestDetermineFixtureHarness_Hydrate_Dehydrate(t *testing.T) {
	src := `package testpkg

import "context"

type PGSharedFixture struct {
	ConnStr string
}

func (f *PGSharedFixture) BeforeAll(ctx context.Context) error   { return nil }
func (f *PGSharedFixture) Hydrate(ctx context.Context) error     { return nil }
func (f *PGSharedFixture) Dehydrate(ctx context.Context) error   { return nil }
`
	fm := loadFixtureWithMethods(t, src)
	spec, err := DetermineFixture(fm.genDecls[0], fm.pkg)
	gotest.NoError(t, err)

	for _, fd := range fm.funcDecls {
		_, err := DetermineFixtureHarness(fd, fm.pkg, spec)
		gotest.NoError(t, err)
	}

	gotest.True(t, spec.Hydrate != nil, "Hydrate should be set")
	gotest.True(t, spec.Dehydrate != nil, "Dehydrate should be set")
	gotest.True(t, spec.HydrateDecl != nil, "HydrateDecl should be set")
	gotest.Equal(t, "Hydrate", spec.HydrateDecl.Name.Name)
}

func TestDetermineFixtureHarness_Hydrate_OnPackageFixture_Rejected(t *testing.T) {
	src := `package testpkg

import "context"

type DBFixture struct {
	Conn string
}

func (f *DBFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *DBFixture) Hydrate(ctx context.Context) error   { return nil }
`
	fm := loadFixtureWithMethods(t, src)
	spec, err := DetermineFixture(fm.genDecls[0], fm.pkg)
	gotest.NoError(t, err)

	for _, fd := range fm.funcDecls {
		pos, err := DetermineFixtureHarness(fd, fm.pkg, spec)
		if fd.Name.Name == "Hydrate" {
			gotest.True(t, err != nil, "Hydrate should be rejected on package fixture")
			gotest.Contains(t, err.Error(), "Hydrate/Dehydrate are for shared fixtures only")
			gotest.True(t, pos > 0)
		}
	}
}

func TestDetermineFixtureHarness_FixtureConfigType_NoInterference(t *testing.T) {
	src := `package testpkg

import "context"

type FixtureConfig struct {
	Timeout int
}

type PGSharedFixture struct {
	ConnStr string
}

func (f *PGSharedFixture) BeforeAll(ctx context.Context) error { return nil }
`
	fm := loadFixtureWithMethods(t, src)

	var spec *FixtureSpec
	for _, gd := range fm.genDecls {
		s, err := DetermineFixture(gd, fm.pkg)
		gotest.NoError(t, err)
		if s != nil {
			spec = s
			break
		}
	}
	gotest.True(t, spec != nil)
	gotest.Equal(t, SharedFixture, spec.Kind)

	for _, fd := range fm.funcDecls {
		_, err := DetermineFixtureHarness(fd, fm.pkg, spec)
		gotest.NoError(t, err)
	}

	gotest.True(t, spec.BeforeAll != nil)
}

func TestDetermineFixtureHarness_Dehydrate_OnPackageFixture_Rejected(t *testing.T) {
	src := `package testpkg

import "context"

type DBFixture struct {
	Conn string
}

func (f *DBFixture) BeforeAll(ctx context.Context) error  { return nil }
func (f *DBFixture) Dehydrate(ctx context.Context) error  { return nil }
`
	fm := loadFixtureWithMethods(t, src)
	spec, err := DetermineFixture(fm.genDecls[0], fm.pkg)
	gotest.NoError(t, err)

	for _, fd := range fm.funcDecls {
		pos, err := DetermineFixtureHarness(fd, fm.pkg, spec)
		if fd.Name.Name == "Dehydrate" {
			gotest.True(t, err != nil, "Dehydrate should be rejected on package fixture")
			gotest.Contains(t, err.Error(), "Hydrate/Dehydrate are for shared fixtures only")
			gotest.True(t, pos > 0)
		}
	}
}

func TestDetermineFixtureHarness_FixtureConfig_OnSharedFixture_Rejected(t *testing.T) {
	src := `package testpkg

import "context"

type PGSharedFixture struct {
	ConnStr string
}

func (f *PGSharedFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *PGSharedFixture) FixtureConfig() int { return 0 }
`
	fm := loadFixtureWithMethods(t, src)
	spec, err := DetermineFixture(fm.genDecls[0], fm.pkg)
	gotest.NoError(t, err)

	for _, fd := range fm.funcDecls {
		pos, err := DetermineFixtureHarness(fd, fm.pkg, spec)
		if fd.Name.Name == "FixtureConfig" {
			gotest.True(t, err != nil, "FixtureConfig should be rejected on shared fixture")
			gotest.Contains(t, err.Error(), "should use SharedFixtureConfig()")
			gotest.True(t, pos > 0)
		}
	}
}

func TestDetermineFixtureHarness_SharedFixtureConfig_OnPackageFixture_Rejected(t *testing.T) {
	src := `package testpkg

import "context"

type DBFixture struct {
	Conn string
}

func (f *DBFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *DBFixture) SharedFixtureConfig() int { return 0 }
`
	fm := loadFixtureWithMethods(t, src)
	spec, err := DetermineFixture(fm.genDecls[0], fm.pkg)
	gotest.NoError(t, err)

	for _, fd := range fm.funcDecls {
		pos, err := DetermineFixtureHarness(fd, fm.pkg, spec)
		if fd.Name.Name == "SharedFixtureConfig" {
			gotest.True(t, err != nil, "SharedFixtureConfig should be rejected on package fixture")
			gotest.Contains(t, err.Error(), "should use FixtureConfig()")
			gotest.True(t, pos > 0)
		}
	}
}

func TestDetermineFixtureHarness_PackageFixture_Lifecycle(t *testing.T) {
	src := `package testpkg

import "context"

type DBFixture struct {
	Conn string
}

func (f *DBFixture) BeforeAll(ctx context.Context) error  { return nil }
func (f *DBFixture) AfterAll(ctx context.Context) error   { return nil }
func (f *DBFixture) BeforeEach(ctx context.Context) error { return nil }
func (f *DBFixture) AfterEach(ctx context.Context) error  { return nil }
`
	fm := loadFixtureWithMethods(t, src)
	spec, err := DetermineFixture(fm.genDecls[0], fm.pkg)
	gotest.NoError(t, err)
	gotest.Equal(t, PackageFixture, spec.Kind)

	for _, fd := range fm.funcDecls {
		_, err := DetermineFixtureHarness(fd, fm.pkg, spec)
		gotest.NoError(t, err)
	}

	gotest.True(t, spec.BeforeAll != nil, "BeforeAll should be set")
	gotest.True(t, spec.AfterAll != nil, "AfterAll should be set")
	gotest.True(t, spec.BeforeEach != nil, "BeforeEach should be set")
	gotest.True(t, spec.AfterEach != nil, "AfterEach should be set")
}

func TestDetermineFixtureHarness_WrongSignature_Rejected(t *testing.T) {
	src := `package testpkg

type DBFixture struct {
	Conn string
}

func (f *DBFixture) BeforeAll() error { return nil }
`
	fm := loadFixtureWithMethods(t, src)
	spec, err := DetermineFixture(fm.genDecls[0], fm.pkg)
	gotest.NoError(t, err)

	for _, fd := range fm.funcDecls {
		pos, err := DetermineFixtureHarness(fd, fm.pkg, spec)
		if fd.Name.Name == "BeforeAll" {
			gotest.True(t, err != nil, "BeforeAll with wrong sig should be rejected")
			gotest.Contains(t, err.Error(), "unsupported signature")
			gotest.True(t, pos > 0)
		}
	}
}

func TestDetermineFixtureHarness_UnexportedMethod_Ignored(t *testing.T) {
	src := `package testpkg

import "context"

type DBFixture struct {
	Conn string
}

func (f *DBFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *DBFixture) helper() {}
`
	fm := loadFixtureWithMethods(t, src)
	spec, err := DetermineFixture(fm.genDecls[0], fm.pkg)
	gotest.NoError(t, err)

	for _, fd := range fm.funcDecls {
		_, err := DetermineFixtureHarness(fd, fm.pkg, spec)
		gotest.NoError(t, err)
	}

	gotest.True(t, spec.BeforeAll != nil)
}

func TestDetermineFixtureHarness_NonMatchingReceiver_Ignored(t *testing.T) {
	src := `package testpkg

import "context"

type DBFixture struct {
	Conn string
}

type OtherFixture struct {
	Addr string
}

func (f *DBFixture) BeforeAll(ctx context.Context) error    { return nil }
func (f *OtherFixture) BeforeAll(ctx context.Context) error { return nil }
`
	fm := loadFixtureWithMethods(t, src)

	var dbSpec *FixtureSpec
	for _, gd := range fm.genDecls {
		s, err := DetermineFixture(gd, fm.pkg)
		gotest.NoError(t, err)
		if s != nil && s.Identifier() == "DBFixture" {
			dbSpec = s
			break
		}
	}
	gotest.True(t, dbSpec != nil)

	for _, fd := range fm.funcDecls {
		_, err := DetermineFixtureHarness(fd, fm.pkg, dbSpec)
		gotest.NoError(t, err)
	}

	gotest.True(t, dbSpec.BeforeAll != nil, "only matching receiver's BeforeAll should be set")
}

func TestExportedFieldNames(t *testing.T) {
	src := `package testpkg

type PGSharedFixture struct {
	ConnStr string
	Port    int
	local   string
}
`
	pkg, decls := loadFixtureAST(t, src)
	spec, err := DetermineFixture(decls[0], pkg)
	gotest.NoError(t, err)

	names := spec.ExportedFieldNames()
	gotest.Equal(t, 2, len(names))
	gotest.Equal(t, "ConnStr", names[0])
	gotest.Equal(t, "Port", names[1])
}

func TestValidateContextErrorSig(t *testing.T) {
	src := `package testpkg

import "context"

type S struct{}

func (s *S) Good(ctx context.Context) error { return nil }
func (s *S) NoParams() error { return nil }
func (s *S) WrongParam(x int) error { return nil }
func (s *S) NoReturn(ctx context.Context) {}
func (s *S) WrongReturn(ctx context.Context) int { return 0 }
func (s *S) TooManyParams(ctx context.Context, x int) error { return nil }
`
	fm := loadFixtureWithMethods(t, src)

	tests := []struct {
		name    string
		wantErr bool
		errMsg  string
	}{
		{"Good", false, ""},
		{"NoParams", true, "unsupported signature"},
		{"WrongParam", true, "unsupported param type"},
		{"NoReturn", true, "unsupported signature"},
		{"WrongReturn", true, "unsupported return type"},
		{"TooManyParams", true, "unsupported signature"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for _, fd := range fm.funcDecls {
				if fd.Name.Name != tc.name {
					continue
				}
				m := fm.pkg.TypesInfo.ObjectOf(fd.Name).(*types.Func)
				sig := fm.pkg.TypesInfo.TypeOf(fd.Name).(*types.Signature)
				methodID := "S." + m.Name()

				err := validateContextErrorSig(sig, methodID)
				if tc.wantErr {
					gotest.True(t, err != nil, "expected error for %s", tc.name)
					gotest.Contains(t, err.Error(), tc.errMsg)
				} else {
					gotest.NoError(t, err)
				}
			}
		})
	}
}
