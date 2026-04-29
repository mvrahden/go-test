package gotestast

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

func buildFixtureSpec(t *testing.T, src string) *FixtureSpec {
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

	var spec *FixtureSpec
	for _, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		s, err := DetermineFixture(gd, pkg)
		gotest.NoError(t, err)
		if s != nil && s.Kind == SharedFixture {
			spec = s
			break
		}
	}
	gotest.True(t, spec != nil, "expected to find SharedFixture")

	for _, d := range f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok {
			continue
		}
		_, err := DetermineFixtureHarness(fd, pkg, spec)
		gotest.NoError(t, err)
	}

	return spec
}

func TestClassifyLocalFields_DirectAssignment(t *testing.T) {
	src := `package testpkg

import "context"

type PGSharedFixture struct {
	ConnStr string
	Port    int
	Pool    int
}

func (f *PGSharedFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *PGSharedFixture) Hydrate(ctx context.Context) error {
	f.Pool = 42
	return nil
}
`
	spec := buildFixtureSpec(t, src)
	local := ClassifyLocalFields(spec)

	gotest.True(t, local != nil)
	gotest.True(t, local["Pool"], "Pool should be local")
	gotest.True(t, !local["ConnStr"], "ConnStr should not be local")
	gotest.True(t, !local["Port"], "Port should not be local")
	gotest.Equal(t, 1, len(local))
}

func TestClassifyLocalFields_HelperMethodAssignment(t *testing.T) {
	src := `package testpkg

import "context"

type PGSharedFixture struct {
	ConnStr string
	Port    int
	Pool    int
}

func (f *PGSharedFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *PGSharedFixture) Hydrate(ctx context.Context) error {
	return f.connect(ctx)
}
func (f *PGSharedFixture) connect(ctx context.Context) error {
	f.Pool = 42
	return nil
}
`
	spec := buildFixtureSpec(t, src)
	local := ClassifyLocalFields(spec)

	gotest.True(t, local != nil)
	gotest.True(t, local["Pool"], "Pool should be local (via connect helper)")
	gotest.Equal(t, 1, len(local))
}

func TestClassifyLocalFields_DirectAndHelper(t *testing.T) {
	src := `package testpkg

import "context"

type PGSharedFixture struct {
	ConnStr string
	Pool    int
	Cache   int
}

func (f *PGSharedFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *PGSharedFixture) Hydrate(ctx context.Context) error {
	f.Cache = 1
	return f.connect(ctx)
}
func (f *PGSharedFixture) connect(ctx context.Context) error {
	f.Pool = 42
	return nil
}
`
	spec := buildFixtureSpec(t, src)
	local := ClassifyLocalFields(spec)

	gotest.True(t, local != nil)
	gotest.True(t, local["Pool"], "Pool should be local")
	gotest.True(t, local["Cache"], "Cache should be local")
	gotest.True(t, !local["ConnStr"], "ConnStr should not be local")
	gotest.Equal(t, 2, len(local))
}

func TestClassifyLocalFields_ConditionalAssignment(t *testing.T) {
	src := `package testpkg

import "context"

type PGSharedFixture struct {
	ConnStr string
	Pool    int
}

func (f *PGSharedFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *PGSharedFixture) Hydrate(ctx context.Context) error {
	if true {
		f.Pool = 42
	}
	return nil
}
`
	spec := buildFixtureSpec(t, src)
	local := ClassifyLocalFields(spec)

	gotest.True(t, local != nil)
	gotest.True(t, local["Pool"], "Pool should be local even inside if block")
}

func TestClassifyLocalFields_MultipleReturnAssignment(t *testing.T) {
	src := `package testpkg

import "context"

type PGSharedFixture struct {
	ConnStr string
	Pool    int
}

func (f *PGSharedFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *PGSharedFixture) Hydrate(ctx context.Context) error {
	var err error
	f.Pool, err = 42, nil
	_ = err
	return nil
}
`
	spec := buildFixtureSpec(t, src)
	local := ClassifyLocalFields(spec)

	gotest.True(t, local != nil)
	gotest.True(t, local["Pool"], "Pool should be local in multi-return assign")
}

func TestClassifyLocalFields_NoHydrate(t *testing.T) {
	src := `package testpkg

import "context"

type PGSharedFixture struct {
	ConnStr string
	Pool    int
}

func (f *PGSharedFixture) BeforeAll(ctx context.Context) error { return nil }
`
	spec := buildFixtureSpec(t, src)
	local := ClassifyLocalFields(spec)

	gotest.True(t, local == nil, "no Hydrate → nil local fields")
}

func TestClassifyLocalFields_HydrateNoAssignments(t *testing.T) {
	src := `package testpkg

import "context"

type PGSharedFixture struct {
	ConnStr string
}

func (f *PGSharedFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *PGSharedFixture) Hydrate(ctx context.Context) error {
	_ = f.ConnStr
	return nil
}
`
	spec := buildFixtureSpec(t, src)
	local := ClassifyLocalFields(spec)

	gotest.True(t, local == nil, "Hydrate reads only → nil local fields")
}

func TestClassifyLocalFields_UnexportedFieldsIgnored(t *testing.T) {
	src := `package testpkg

import "context"

type PGSharedFixture struct {
	ConnStr string
	pool    int
}

func (f *PGSharedFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *PGSharedFixture) Hydrate(ctx context.Context) error {
	f.pool = 42
	return nil
}
`
	spec := buildFixtureSpec(t, src)
	local := ClassifyLocalFields(spec)

	gotest.True(t, local == nil, "unexported field assignments should be ignored")
}

func TestClassifyLocalFields_ForLoopAssignment(t *testing.T) {
	src := `package testpkg

import "context"

type PGSharedFixture struct {
	ConnStr string
	Pool    int
}

func (f *PGSharedFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *PGSharedFixture) Hydrate(ctx context.Context) error {
	for i := 0; i < 1; i++ {
		f.Pool = 42
	}
	return nil
}
`
	spec := buildFixtureSpec(t, src)
	local := ClassifyLocalFields(spec)

	gotest.True(t, local != nil)
	gotest.True(t, local["Pool"], "Pool should be local inside for loop")
}

func TestClassifyLocalFields_SwitchAssignment(t *testing.T) {
	src := `package testpkg

import "context"

type PGSharedFixture struct {
	ConnStr string
	Pool    int
}

func (f *PGSharedFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *PGSharedFixture) Hydrate(ctx context.Context) error {
	switch {
	case true:
		f.Pool = 42
	}
	return nil
}
`
	spec := buildFixtureSpec(t, src)
	local := ClassifyLocalFields(spec)

	gotest.True(t, local != nil)
	gotest.True(t, local["Pool"], "Pool should be local inside switch")
}
