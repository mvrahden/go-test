package gotestast_test

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"github.com/mvrahden/go-test/internal/gotestast"
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

func buildFixtureSpec(t *testing.T, src string) *gotestast.FixtureSpec {
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

	var spec *gotestast.FixtureSpec
	for _, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		s, err := gotestast.DetermineFixture(gd, pkg)
		gotest.NoError(t, err)
		if s != nil && s.Kind == gotestast.SharedFixture {
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
		_, err := gotestast.DetermineFixtureHarness(fd, pkg, spec)
		gotest.NoError(t, err)
	}

	return spec
}

// GotestastTestSuite tests AST-level fixture and suite detection,
// harness binding, and lifecycle method validation.
type GotestastTestSuite struct{}

func (s *GotestastTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *GotestastTestSuite) TestDetermineFixture(t *gotest.T) {
	t.When("type ends with Fixture suffix and is a struct", func(w *gotest.T) {
		w.It("returns PackageFixture for non-shared fixture", func(it *gotest.T) {
			src := `package testpkg

type DBFixture struct {
	Conn string
}
`
			pkg, decls := loadFixtureAST(it.T(), src)
			gotest.Equal(it, 1, len(decls))

			spec, err := gotestast.DetermineFixture(decls[0], pkg)
			gotest.NoError(it, err)
			gotest.True(it, spec != nil, "expected non-nil FixtureSpec")
			gotest.Equal(it, gotestast.PackageFixture, spec.Kind)
			gotest.Equal(it, "DBFixture", spec.Identifier())
			gotest.Equal(it, "testpkg", spec.PackageName())
		})

		w.It("returns SharedFixture for type ending with SharedFixture", func(it *gotest.T) {
			src := `package testpkg

type RedisSharedFixture struct {
	Addr string
}
`
			pkg, decls := loadFixtureAST(it.T(), src)
			gotest.Equal(it, 1, len(decls))

			spec, err := gotestast.DetermineFixture(decls[0], pkg)
			gotest.NoError(it, err)
			gotest.True(it, spec != nil, "expected non-nil FixtureSpec")
			gotest.Equal(it, gotestast.SharedFixture, spec.Kind)
			gotest.Equal(it, "RedisSharedFixture", spec.Identifier())
		})
	})

	t.When("type does not end with Fixture suffix", func(w *gotest.T) {
		w.It("returns nil spec", func(it *gotest.T) {
			src := `package testpkg

type PlainStruct struct {
	Name string
}
`
			pkg, decls := loadFixtureAST(it.T(), src)
			gotest.Equal(it, 1, len(decls))

			spec, err := gotestast.DetermineFixture(decls[0], pkg)
			gotest.NoError(it, err)
			gotest.True(it, spec == nil, "expected nil FixtureSpec for struct without Fixture suffix")
		})
	})

	t.When("type ends with Fixture but is not a struct", func(w *gotest.T) {
		w.It("returns an error", func(it *gotest.T) {
			src := `package testpkg

type BadFixture int
`
			pkg, decls := loadFixtureAST(it.T(), src)
			gotest.Equal(it, 1, len(decls))

			spec, err := gotestast.DetermineFixture(decls[0], pkg)
			gotest.True(it, err != nil, "expected error for non-struct fixture")
			gotest.True(it, spec == nil, "expected nil FixtureSpec on error")
			gotest.Contains(it, err.Error(), "fixture must be a struct type")
		})
	})
}

func (s *GotestastTestSuite) TestDetermineFixtureHarness(t *gotest.T) {
	t.When("shared fixture has BeforeAll and AfterAll", func(w *gotest.T) {
		w.It("sets lifecycle methods", func(it *gotest.T) {
			src := `package testpkg

import "context"

type PGSharedFixture struct {
	ConnStr string
}

func (f *PGSharedFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *PGSharedFixture) AfterAll(ctx context.Context) error  { return nil }
`
			fm := loadFixtureWithMethods(it.T(), src)
			gotest.Equal(it, 1, len(fm.genDecls))
			gotest.Equal(it, 2, len(fm.funcDecls))

			spec, err := gotestast.DetermineFixture(fm.genDecls[0], fm.pkg)
			gotest.NoError(it, err)
			gotest.True(it, spec != nil)
			gotest.Equal(it, gotestast.SharedFixture, spec.Kind)

			for _, fd := range fm.funcDecls {
				_, err := gotestast.DetermineFixtureHarness(fd, fm.pkg, spec)
				gotest.NoError(it, err)
			}

			gotest.True(it, spec.BeforeAll != nil, "BeforeAll should be set")
			gotest.True(it, spec.AfterAll != nil, "AfterAll should be set")
		})
	})

	t.When("shared fixture has BeforeEach", func(w *gotest.T) {
		w.It("rejects it", func(it *gotest.T) {
			src := `package testpkg

import "context"

type BadSharedFixture struct {
	Addr string
}

func (f *BadSharedFixture) BeforeAll(ctx context.Context) error  { return nil }
func (f *BadSharedFixture) BeforeEach(ctx context.Context) error { return nil }
`
			fm := loadFixtureWithMethods(it.T(), src)
			spec, err := gotestast.DetermineFixture(fm.genDecls[0], fm.pkg)
			gotest.NoError(it, err)

			for _, fd := range fm.funcDecls {
				pos, err := gotestast.DetermineFixtureHarness(fd, fm.pkg, spec)
				if fd.Name.Name == "BeforeEach" {
					gotest.True(it, err != nil, "BeforeEach should be rejected on shared fixture")
					gotest.Contains(it, err.Error(), "must not have BeforeEach method")
					gotest.True(it, pos > 0, "error position should be set")
				}
			}
		})
	})

	t.When("shared fixture has AfterEach", func(w *gotest.T) {
		w.It("rejects it", func(it *gotest.T) {
			src := `package testpkg

import "context"

type BadSharedFixture struct {
	Addr string
}

func (f *BadSharedFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *BadSharedFixture) AfterEach(ctx context.Context) error { return nil }
`
			fm := loadFixtureWithMethods(it.T(), src)
			spec, err := gotestast.DetermineFixture(fm.genDecls[0], fm.pkg)
			gotest.NoError(it, err)

			for _, fd := range fm.funcDecls {
				pos, err := gotestast.DetermineFixtureHarness(fd, fm.pkg, spec)
				if fd.Name.Name == "AfterEach" {
					gotest.True(it, err != nil, "AfterEach should be rejected on shared fixture")
					gotest.Contains(it, err.Error(), "must not have AfterEach method")
					gotest.True(it, pos > 0, "error position should be set")
				}
			}
		})
	})

	t.When("shared fixture has Hydrate and Dehydrate", func(w *gotest.T) {
		w.It("sets both methods and HydrateDecl", func(it *gotest.T) {
			src := `package testpkg

import "context"

type PGSharedFixture struct {
	ConnStr string
}

func (f *PGSharedFixture) BeforeAll(ctx context.Context) error   { return nil }
func (f *PGSharedFixture) Hydrate(ctx context.Context) error     { return nil }
func (f *PGSharedFixture) Dehydrate(ctx context.Context) error   { return nil }
`
			fm := loadFixtureWithMethods(it.T(), src)
			spec, err := gotestast.DetermineFixture(fm.genDecls[0], fm.pkg)
			gotest.NoError(it, err)

			for _, fd := range fm.funcDecls {
				_, err := gotestast.DetermineFixtureHarness(fd, fm.pkg, spec)
				gotest.NoError(it, err)
			}

			gotest.True(it, spec.Hydrate != nil, "Hydrate should be set")
			gotest.True(it, spec.Dehydrate != nil, "Dehydrate should be set")
			gotest.True(it, spec.HydrateDecl != nil, "HydrateDecl should be set")
			gotest.Equal(it, "Hydrate", spec.HydrateDecl.Name.Name)
		})
	})

	t.When("package fixture has Hydrate", func(w *gotest.T) {
		w.It("rejects it", func(it *gotest.T) {
			src := `package testpkg

import "context"

type DBFixture struct {
	Conn string
}

func (f *DBFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *DBFixture) Hydrate(ctx context.Context) error   { return nil }
`
			fm := loadFixtureWithMethods(it.T(), src)
			spec, err := gotestast.DetermineFixture(fm.genDecls[0], fm.pkg)
			gotest.NoError(it, err)

			for _, fd := range fm.funcDecls {
				pos, err := gotestast.DetermineFixtureHarness(fd, fm.pkg, spec)
				if fd.Name.Name == "Hydrate" {
					gotest.True(it, err != nil, "Hydrate should be rejected on package fixture")
					gotest.Contains(it, err.Error(), "Hydrate/Dehydrate are for shared fixtures only")
					gotest.True(it, pos > 0)
				}
			}
		})
	})

	t.When("package fixture has Dehydrate", func(w *gotest.T) {
		w.It("rejects it", func(it *gotest.T) {
			src := `package testpkg

import "context"

type DBFixture struct {
	Conn string
}

func (f *DBFixture) BeforeAll(ctx context.Context) error  { return nil }
func (f *DBFixture) Dehydrate(ctx context.Context) error  { return nil }
`
			fm := loadFixtureWithMethods(it.T(), src)
			spec, err := gotestast.DetermineFixture(fm.genDecls[0], fm.pkg)
			gotest.NoError(it, err)

			for _, fd := range fm.funcDecls {
				pos, err := gotestast.DetermineFixtureHarness(fd, fm.pkg, spec)
				if fd.Name.Name == "Dehydrate" {
					gotest.True(it, err != nil, "Dehydrate should be rejected on package fixture")
					gotest.Contains(it, err.Error(), "Hydrate/Dehydrate are for shared fixtures only")
					gotest.True(it, pos > 0)
				}
			}
		})
	})

	t.When("FixtureConfig type exists alongside shared fixture", func(w *gotest.T) {
		w.It("does not interfere with fixture detection", func(it *gotest.T) {
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
			fm := loadFixtureWithMethods(it.T(), src)

			var spec *gotestast.FixtureSpec
			for _, gd := range fm.genDecls {
				s, err := gotestast.DetermineFixture(gd, fm.pkg)
				gotest.NoError(it, err)
				if s != nil {
					spec = s
					break
				}
			}
			gotest.True(it, spec != nil)
			gotest.Equal(it, gotestast.SharedFixture, spec.Kind)

			for _, fd := range fm.funcDecls {
				_, err := gotestast.DetermineFixtureHarness(fd, fm.pkg, spec)
				gotest.NoError(it, err)
			}

			gotest.True(it, spec.BeforeAll != nil)
		})
	})

	t.When("shared fixture has FixtureConfig method", func(w *gotest.T) {
		w.It("rejects it and suggests SharedFixtureConfig", func(it *gotest.T) {
			src := `package testpkg

import "context"

type PGSharedFixture struct {
	ConnStr string
}

func (f *PGSharedFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *PGSharedFixture) FixtureConfig() int { return 0 }
`
			fm := loadFixtureWithMethods(it.T(), src)
			spec, err := gotestast.DetermineFixture(fm.genDecls[0], fm.pkg)
			gotest.NoError(it, err)

			for _, fd := range fm.funcDecls {
				pos, err := gotestast.DetermineFixtureHarness(fd, fm.pkg, spec)
				if fd.Name.Name == "FixtureConfig" {
					gotest.True(it, err != nil, "FixtureConfig should be rejected on shared fixture")
					gotest.Contains(it, err.Error(), "should use SharedFixtureConfig()")
					gotest.True(it, pos > 0)
				}
			}
		})
	})

	t.When("package fixture has SharedFixtureConfig method", func(w *gotest.T) {
		w.It("rejects it and suggests FixtureConfig", func(it *gotest.T) {
			src := `package testpkg

import "context"

type DBFixture struct {
	Conn string
}

func (f *DBFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *DBFixture) SharedFixtureConfig() int { return 0 }
`
			fm := loadFixtureWithMethods(it.T(), src)
			spec, err := gotestast.DetermineFixture(fm.genDecls[0], fm.pkg)
			gotest.NoError(it, err)

			for _, fd := range fm.funcDecls {
				pos, err := gotestast.DetermineFixtureHarness(fd, fm.pkg, spec)
				if fd.Name.Name == "SharedFixtureConfig" {
					gotest.True(it, err != nil, "SharedFixtureConfig should be rejected on package fixture")
					gotest.Contains(it, err.Error(), "should use FixtureConfig()")
					gotest.True(it, pos > 0)
				}
			}
		})
	})

	t.When("package fixture has all lifecycle methods", func(w *gotest.T) {
		w.It("sets BeforeAll, AfterAll, BeforeEach, AfterEach", func(it *gotest.T) {
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
			fm := loadFixtureWithMethods(it.T(), src)
			spec, err := gotestast.DetermineFixture(fm.genDecls[0], fm.pkg)
			gotest.NoError(it, err)
			gotest.Equal(it, gotestast.PackageFixture, spec.Kind)

			for _, fd := range fm.funcDecls {
				_, err := gotestast.DetermineFixtureHarness(fd, fm.pkg, spec)
				gotest.NoError(it, err)
			}

			gotest.True(it, spec.BeforeAll != nil, "BeforeAll should be set")
			gotest.True(it, spec.AfterAll != nil, "AfterAll should be set")
			gotest.True(it, spec.BeforeEach != nil, "BeforeEach should be set")
			gotest.True(it, spec.AfterEach != nil, "AfterEach should be set")
		})
	})

	t.When("method has wrong signature", func(w *gotest.T) {
		w.It("rejects it", func(it *gotest.T) {
			src := `package testpkg

type DBFixture struct {
	Conn string
}

func (f *DBFixture) BeforeAll() error { return nil }
`
			fm := loadFixtureWithMethods(it.T(), src)
			spec, err := gotestast.DetermineFixture(fm.genDecls[0], fm.pkg)
			gotest.NoError(it, err)

			for _, fd := range fm.funcDecls {
				pos, err := gotestast.DetermineFixtureHarness(fd, fm.pkg, spec)
				if fd.Name.Name == "BeforeAll" {
					gotest.True(it, err != nil, "BeforeAll with wrong sig should be rejected")
					gotest.Contains(it, err.Error(), "unsupported signature")
					gotest.True(it, pos > 0)
				}
			}
		})
	})

	t.When("fixture has unexported method", func(w *gotest.T) {
		w.It("ignores it", func(it *gotest.T) {
			src := `package testpkg

import "context"

type DBFixture struct {
	Conn string
}

func (f *DBFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *DBFixture) helper() {}
`
			fm := loadFixtureWithMethods(it.T(), src)
			spec, err := gotestast.DetermineFixture(fm.genDecls[0], fm.pkg)
			gotest.NoError(it, err)

			for _, fd := range fm.funcDecls {
				_, err := gotestast.DetermineFixtureHarness(fd, fm.pkg, spec)
				gotest.NoError(it, err)
			}

			gotest.True(it, spec.BeforeAll != nil)
		})
	})

	t.When("method has non-matching receiver", func(w *gotest.T) {
		w.It("ignores it", func(it *gotest.T) {
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
			fm := loadFixtureWithMethods(it.T(), src)

			var dbSpec *gotestast.FixtureSpec
			for _, gd := range fm.genDecls {
				s, err := gotestast.DetermineFixture(gd, fm.pkg)
				gotest.NoError(it, err)
				if s != nil && s.Identifier() == "DBFixture" {
					dbSpec = s
					break
				}
			}
			gotest.True(it, dbSpec != nil)

			for _, fd := range fm.funcDecls {
				_, err := gotestast.DetermineFixtureHarness(fd, fm.pkg, dbSpec)
				gotest.NoError(it, err)
			}

			gotest.True(it, dbSpec.BeforeAll != nil, "only matching receiver's BeforeAll should be set")
		})
	})
}

func (s *GotestastTestSuite) TestFixtureValidation(t *gotest.T) {
	t.When("Hydrate present without Dehydrate", func(w *gotest.T) {
		w.It("returns an error", func(it *gotest.T) {
			src := `package testpkg

import "context"

type PGSharedFixture struct {
	ConnStr string
}

func (f *PGSharedFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *PGSharedFixture) Hydrate(ctx context.Context) error   { return nil }
`
			fm := loadFixtureWithMethods(it.T(), src)
			spec, err := gotestast.DetermineFixture(fm.genDecls[0], fm.pkg)
			gotest.NoError(it, err)

			for _, fd := range fm.funcDecls {
				_, err := gotestast.DetermineFixtureHarness(fd, fm.pkg, spec)
				gotest.NoError(it, err)
			}

			pos, err := gotestast.ValidateFixtureConsistency(spec)
			gotest.True(it, err != nil, "expected error for Hydrate without Dehydrate")
			gotest.Contains(it, err.Error(), "has Hydrate but no Dehydrate")
			gotest.True(it, pos > 0)
		})
	})

	t.When("Dehydrate present without Hydrate", func(w *gotest.T) {
		w.It("returns an error", func(it *gotest.T) {
			src := `package testpkg

import "context"

type PGSharedFixture struct {
	ConnStr string
}

func (f *PGSharedFixture) BeforeAll(ctx context.Context) error   { return nil }
func (f *PGSharedFixture) Dehydrate(ctx context.Context) error   { return nil }
`
			fm := loadFixtureWithMethods(it.T(), src)
			spec, err := gotestast.DetermineFixture(fm.genDecls[0], fm.pkg)
			gotest.NoError(it, err)

			for _, fd := range fm.funcDecls {
				_, err := gotestast.DetermineFixtureHarness(fd, fm.pkg, spec)
				gotest.NoError(it, err)
			}

			pos, err := gotestast.ValidateFixtureConsistency(spec)
			gotest.True(it, err != nil, "expected error for Dehydrate without Hydrate")
			gotest.Contains(it, err.Error(), "has Dehydrate but no Hydrate")
			gotest.True(it, pos > 0)
		})
	})

	t.When("both Hydrate and Dehydrate are present", func(w *gotest.T) {
		w.It("passes validation", func(it *gotest.T) {
			src := `package testpkg

import "context"

type PGSharedFixture struct {
	ConnStr string
}

func (f *PGSharedFixture) BeforeAll(ctx context.Context) error   { return nil }
func (f *PGSharedFixture) Hydrate(ctx context.Context) error     { return nil }
func (f *PGSharedFixture) Dehydrate(ctx context.Context) error   { return nil }
`
			fm := loadFixtureWithMethods(it.T(), src)
			spec, err := gotestast.DetermineFixture(fm.genDecls[0], fm.pkg)
			gotest.NoError(it, err)

			for _, fd := range fm.funcDecls {
				_, err := gotestast.DetermineFixtureHarness(fd, fm.pkg, spec)
				gotest.NoError(it, err)
			}

			pos, err := gotestast.ValidateFixtureConsistency(spec)
			gotest.NoError(it, err)
			gotest.Equal(it, token.Pos(-1), pos)
		})
	})

	t.When("fixture is a PackageFixture", func(w *gotest.T) {
		w.It("skips Hydrate/Dehydrate check", func(it *gotest.T) {
			src := `package testpkg

import "context"

type DBFixture struct {
	Conn string
}

func (f *DBFixture) BeforeAll(ctx context.Context) error { return nil }
`
			fm := loadFixtureWithMethods(it.T(), src)
			spec, err := gotestast.DetermineFixture(fm.genDecls[0], fm.pkg)
			gotest.NoError(it, err)

			for _, fd := range fm.funcDecls {
				_, err := gotestast.DetermineFixtureHarness(fd, fm.pkg, spec)
				gotest.NoError(it, err)
			}

			pos, err := gotestast.ValidateFixtureConsistency(spec)
			gotest.NoError(it, err)
			gotest.Equal(it, token.Pos(-1), pos)
		})
	})

	t.When("context-error signature", func(w *gotest.T) {
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
		fm := loadFixtureWithMethods(t.T(), src)

		for sub, tc := range gotest.Each(w, []struct {
			Name    string
			wantErr bool
			errMsg  string
		}{
			{"Good", false, ""},
			{"NoParams", true, "unsupported signature"},
			{"WrongParam", true, "unsupported param type"},
			{"NoReturn", true, "unsupported signature"},
			{"WrongReturn", true, "unsupported return type"},
			{"TooManyParams", true, "unsupported signature"},
		}) {
			for _, fd := range fm.funcDecls {
				if fd.Name.Name != tc.Name {
					continue
				}
				m := fm.pkg.TypesInfo.ObjectOf(fd.Name).(*types.Func)
				sig := fm.pkg.TypesInfo.TypeOf(fd.Name).(*types.Signature)
				methodID := "S." + m.Name()

				err := gotestast.ExportValidateContextErrorSig(sig, methodID)
				if tc.wantErr {
					gotest.True(sub, err != nil, "expected error for %s", tc.Name)
					gotest.Contains(sub, err.Error(), tc.errMsg)
				} else {
					gotest.NoError(sub, err)
				}
			}
		}
	})
}

func (s *GotestastTestSuite) TestExportedFieldNames(t *gotest.T) {
	t.When("fixture has exported and unexported fields", func(w *gotest.T) {
		w.It("returns only exported field names", func(it *gotest.T) {
			src := `package testpkg

type PGSharedFixture struct {
	ConnStr string
	Port    int
	local   string
}
`
			pkg, decls := loadFixtureAST(it.T(), src)
			spec, err := gotestast.DetermineFixture(decls[0], pkg)
			gotest.NoError(it, err)

			names := spec.ExportedFieldNames()
			gotest.Equal(it, 2, len(names))
			gotest.Equal(it, "ConnStr", names[0])
			gotest.Equal(it, "Port", names[1])
		})
	})
}

func (s *GotestastTestSuite) TestClassifyLocalFields(t *gotest.T) {
	t.When("Hydrate directly assigns a field", func(w *gotest.T) {
		w.It("classifies it as local", func(it *gotest.T) {
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
			spec := buildFixtureSpec(it.T(), src)
			local := gotestast.ClassifyLocalFields(spec)

			gotest.True(it, local != nil)
			gotest.True(it, local["Pool"], "Pool should be local")
			gotest.True(it, !local["ConnStr"], "ConnStr should not be local")
			gotest.True(it, !local["Port"], "Port should not be local")
			gotest.Equal(it, 1, len(local))
		})
	})

	t.When("Hydrate assigns via helper method", func(w *gotest.T) {
		w.It("classifies it as local", func(it *gotest.T) {
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
			spec := buildFixtureSpec(it.T(), src)
			local := gotestast.ClassifyLocalFields(spec)

			gotest.True(it, local != nil)
			gotest.True(it, local["Pool"], "Pool should be local (via connect helper)")
			gotest.Equal(it, 1, len(local))
		})
	})

	t.When("Hydrate assigns directly and via helper", func(w *gotest.T) {
		w.It("classifies both as local", func(it *gotest.T) {
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
			spec := buildFixtureSpec(it.T(), src)
			local := gotestast.ClassifyLocalFields(spec)

			gotest.True(it, local != nil)
			gotest.True(it, local["Pool"], "Pool should be local")
			gotest.True(it, local["Cache"], "Cache should be local")
			gotest.True(it, !local["ConnStr"], "ConnStr should not be local")
			gotest.Equal(it, 2, len(local))
		})
	})

	t.When("Hydrate assigns inside if block", func(w *gotest.T) {
		w.It("classifies it as local", func(it *gotest.T) {
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
			spec := buildFixtureSpec(it.T(), src)
			local := gotestast.ClassifyLocalFields(spec)

			gotest.True(it, local != nil)
			gotest.True(it, local["Pool"], "Pool should be local even inside if block")
		})
	})

	t.When("Hydrate assigns in multi-return assignment", func(w *gotest.T) {
		w.It("classifies it as local", func(it *gotest.T) {
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
			spec := buildFixtureSpec(it.T(), src)
			local := gotestast.ClassifyLocalFields(spec)

			gotest.True(it, local != nil)
			gotest.True(it, local["Pool"], "Pool should be local in multi-return assign")
		})
	})

	t.When("fixture has no Hydrate method", func(w *gotest.T) {
		w.It("returns nil", func(it *gotest.T) {
			src := `package testpkg

import "context"

type PGSharedFixture struct {
	ConnStr string
	Pool    int
}

func (f *PGSharedFixture) BeforeAll(ctx context.Context) error { return nil }
`
			spec := buildFixtureSpec(it.T(), src)
			local := gotestast.ClassifyLocalFields(spec)

			gotest.True(it, local == nil, "no Hydrate -> nil local fields")
		})
	})

	t.When("Hydrate has no assignments", func(w *gotest.T) {
		w.It("returns nil", func(it *gotest.T) {
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
			spec := buildFixtureSpec(it.T(), src)
			local := gotestast.ClassifyLocalFields(spec)

			gotest.True(it, local == nil, "Hydrate reads only -> nil local fields")
		})
	})

	t.When("Hydrate assigns unexported fields", func(w *gotest.T) {
		w.It("ignores them", func(it *gotest.T) {
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
			spec := buildFixtureSpec(it.T(), src)
			local := gotestast.ClassifyLocalFields(spec)

			gotest.True(it, local == nil, "unexported field assignments should be ignored")
		})
	})

	t.When("Hydrate assigns inside for loop", func(w *gotest.T) {
		w.It("classifies it as local", func(it *gotest.T) {
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
			spec := buildFixtureSpec(it.T(), src)
			local := gotestast.ClassifyLocalFields(spec)

			gotest.True(it, local != nil)
			gotest.True(it, local["Pool"], "Pool should be local inside for loop")
		})
	})

	t.When("Hydrate assigns inside switch", func(w *gotest.T) {
		w.It("classifies it as local", func(it *gotest.T) {
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
			spec := buildFixtureSpec(it.T(), src)
			local := gotestast.ClassifyLocalFields(spec)

			gotest.True(it, local != nil)
			gotest.True(it, local["Pool"], "Pool should be local inside switch")
		})
	})

	t.When("assignment is two levels deep", func(w *gotest.T) {
		w.It("does not classify it as local", func(it *gotest.T) {
			src := `package testpkg

import "context"

type PGSharedFixture struct {
	ConnStr string
	Pool    int
}

func (f *PGSharedFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *PGSharedFixture) Hydrate(ctx context.Context) error {
	return f.setupA(ctx)
}
func (f *PGSharedFixture) setupA(ctx context.Context) error {
	return f.setupB(ctx)
}
func (f *PGSharedFixture) setupB(ctx context.Context) error {
	f.Pool = 42
	return nil
}
`
			spec := buildFixtureSpec(it.T(), src)
			local := gotestast.ClassifyLocalFields(spec)

			gotest.True(it, local == nil, "two-level-deep assignments should NOT be classified as local")
		})
	})
}

// SpecTestSuite tests TestSuiteSpecSet.ReduceToEffectiveSet, DetermineTestSuite,
// DetermineTestSuiteHarness, and TestSuiteSpec accessor methods.
type SpecTestSuite struct{}

func (s *SpecTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *SpecTestSuite) TestReduceToEffectiveSet(t *gotest.T) {
	t.When("no focused or excluded suites", func(w *gotest.T) {
		w.It("returns all suites as effective", func(it *gotest.T) {
			suites := gotestast.TestSuiteSpecSet{
				gotestast.NewTestSuiteSpecForTest("AlphaTestSuite", "pkg", false),
				gotestast.NewTestSuiteSpecForTest("BetaTestSuite", "pkg", false),
			}
			effective, skippedSuites, skippedCases := suites.ReduceToEffectiveSet()
			gotest.Equal(it, 2, len(effective))
			gotest.Empty(it, skippedSuites)
			gotest.Equal(it, 2, len(skippedCases)) // entries exist but with nil values
		})
	})

	t.When("focused suite present", func(w *gotest.T) {
		w.It("keeps only focused suites, skips unfocused", func(it *gotest.T) {
			suites := gotestast.TestSuiteSpecSet{
				gotestast.NewTestSuiteSpecForTest("F_FocusedTestSuite", "pkg", false),
				gotestast.NewTestSuiteSpecForTest("NormalTestSuite", "pkg", false),
			}
			effective, skippedSuites, _ := suites.ReduceToEffectiveSet()
			gotest.Equal(it, 1, len(effective))
			gotest.Equal(it, "F_FocusedTestSuite", effective[0].Identifier())
			gotest.Equal(it, 1, len(skippedSuites))
			gotest.Equal(it, "NormalTestSuite", skippedSuites[0].Identifier())
		})
	})

	t.When("excluded suite present", func(w *gotest.T) {
		w.It("removes excluded suites", func(it *gotest.T) {
			suites := gotestast.TestSuiteSpecSet{
				gotestast.NewTestSuiteSpecForTest("X_ExcludedTestSuite", "pkg", false),
				gotestast.NewTestSuiteSpecForTest("NormalTestSuite", "pkg", false),
			}
			effective, skippedSuites, _ := suites.ReduceToEffectiveSet()
			gotest.Equal(it, 1, len(effective))
			gotest.Equal(it, "NormalTestSuite", effective[0].Identifier())
			gotest.Equal(it, 1, len(skippedSuites))
		})
	})
}

func (s *SpecTestSuite) TestDetermineTestSuite(t *gotest.T) {
	t.When("valid test suite type", func(w *gotest.T) {
		w.It("detects struct type ending in TestSuite", func(it *gotest.T) {
			pkg, genDecls := loadFixtureAST(it.T(), `
				package testpkg
				type MyTestSuite struct{ Value string }
			`)
			spec, _, err := gotestast.DetermineTestSuite(genDecls[0], pkg)
			gotest.NoError(it, err)
			gotest.True(it, spec != nil)
			gotest.Equal(it, "MyTestSuite", spec.Identifier())
		})
	})

	t.When("non-suite type", func(w *gotest.T) {
		w.It("returns nil for types not ending in TestSuite", func(it *gotest.T) {
			pkg, genDecls := loadFixtureAST(it.T(), `
				package testpkg
				type MyService struct{}
			`)
			spec, _, err := gotestast.DetermineTestSuite(genDecls[0], pkg)
			gotest.NoError(it, err)
			gotest.True(it, spec == nil)
		})
	})

	t.When("generated suite wrapper", func(w *gotest.T) {
		w.It("ignores ƒƒ_GOTEST_ prefixed types", func(it *gotest.T) {
			gotest.False(it, gotestast.IS_TEST_SUITE.MatchString("ƒƒ_GOTEST_MyTestSuite"))
			gotest.True(it, gotestast.IS_TEST_SUITE.MatchString("MyTestSuite"))
		})
	})

	t.When("focused and excluded prefixes", func(w *gotest.T) {
		w.It("accepts F_ prefix as focused", func(it *gotest.T) {
			gotest.True(it, gotestast.IS_TEST_SUITE.MatchString("F_MyTestSuite"))
		})
		w.It("accepts X_ prefix as excluded", func(it *gotest.T) {
			gotest.True(it, gotestast.IS_TEST_SUITE.MatchString("X_MyTestSuite"))
		})
		w.It("rejects underscore-only prefix", func(it *gotest.T) {
			gotest.False(it, gotestast.IS_TEST_SUITE.MatchString("_PrivateTestSuite"))
		})
	})
}

func (s *SpecTestSuite) TestDetermineTestSuiteHarness(t *gotest.T) {
	t.When("lifecycle methods", func(w *gotest.T) {
		w.It("discovers BeforeAll and AfterAll", func(it *gotest.T) {
			fm := loadFixtureWithMethods(it.T(), `
				package testpkg
				import "testing"
				type MyTestSuite struct{}
				func (s *MyTestSuite) BeforeAll(t *testing.T) {}
				func (s *MyTestSuite) AfterAll(t *testing.T) {}
				func (s *MyTestSuite) TestOne(t *testing.T) {}
			`)
			spec, _, err := gotestast.DetermineTestSuite(fm.genDecls[0], fm.pkg)
			gotest.NoError(it, err)
			gotest.True(it, spec != nil)

			for _, fd := range fm.funcDecls {
				_, err := gotestast.DetermineTestSuiteHarness(fd, fm.pkg, spec)
				gotest.NoError(it, err)
			}

			gotest.True(it, spec.BeforeAll() != nil, "BeforeAll should be detected")
			gotest.True(it, spec.AfterAll() != nil, "AfterAll should be detected")
			gotest.Equal(it, 1, len(spec.TestCases()))
		})
	})

	t.When("test case methods", func(w *gotest.T) {
		w.It("detects focused and excluded test cases", func(it *gotest.T) {
			gotest.True(it, gotestast.IS_TEST_CASE.MatchString("TestSomething"))
			gotest.True(it, gotestast.IS_TEST_CASE.MatchString("F_TestFocused"))
			gotest.True(it, gotestast.IS_TEST_CASE.MatchString("X_TestExcluded"))
			gotest.False(it, gotestast.IS_TEST_CASE.MatchString("NotATest"))
		})
	})
}

func (s *SpecTestSuite) TestTestSuiteSpecAccessors(t *gotest.T) {
	t.When("accessor methods", func(w *gotest.T) {
		w.It("returns correct identifiers", func(it *gotest.T) {
			spec := gotestast.NewTestSuiteSpecForTest("MyTestSuite", "mypkg", false)
			gotest.Equal(it, "MyTestSuite", spec.Identifier())
			gotest.Equal(it, "mypkg", spec.PackageName())
			gotest.False(it, spec.IsFocused())
			gotest.False(it, spec.IsExcluded())
			gotest.False(it, spec.IsGenericAlias())
		})
		w.It("detects focused prefix", func(it *gotest.T) {
			spec := gotestast.NewTestSuiteSpecForTest("F_MyTestSuite", "mypkg", false)
			gotest.True(it, spec.IsFocused())
			gotest.False(it, spec.IsExcluded())
		})
		w.It("detects excluded prefix", func(it *gotest.T) {
			spec := gotestast.NewTestSuiteSpecForTest("X_MyTestSuite", "mypkg", false)
			gotest.False(it, spec.IsFocused())
			gotest.True(it, spec.IsExcluded())
		})
		w.It("detects generic alias", func(it *gotest.T) {
			spec := gotestast.NewTestSuiteSpecForTest("MyTestSuite", "mypkg", true)
			gotest.True(it, spec.IsGenericAlias())
		})
		w.It("detects pxtest package", func(it *gotest.T) {
			spec := gotestast.NewTestSuiteSpecForTest("MyTestSuite", "mypkg_test", false)
			gotest.True(it, spec.IsPxTestSuite())
		})
		w.It("detects internal test package", func(it *gotest.T) {
			spec := gotestast.NewTestSuiteSpecForTest("MyTestSuite", "mypkg", false)
			gotest.False(it, spec.IsPxTestSuite())
		})
	})
}

func (s *GotestastTestSuite) TestRegexp(t *gotest.T) {
	testCases := []struct {
		desc string
		fn   *gotestast.ExportRegexpW
		in   []string
		out  []string
	}{
		{"test case rejects unexported names", gotestast.IS_TEST_CASE, []string{"TEST", "Test", "_Test", "ABCTestABC"}, nil},
		{"test case rejects lowercase prefix", gotestast.IS_TEST_CASE, []string{"x_TestFoo", "f_TestFoo"}, nil},
		{"test case matches exported names", gotestast.IS_TEST_CASE, []string{"X_TestFoo", "F_TestFoo", "TestFoo"}, []string{"X_TestFoo", "F_TestFoo", "TestFoo"}},
		{"suite method rejects unexported names", gotestast.IS_TEST_SUITE_METHOD, []string{"TEST", "Test", "_Test", "ABCTestABC"}, nil},
		{"suite method rejects lowercase prefix", gotestast.IS_TEST_SUITE_METHOD, []string{"x_TestFoo", "f_TestFoo"}, nil},
		{"suite method matches exported names", gotestast.IS_TEST_SUITE_METHOD, []string{"X_TestFoo", "F_TestFoo", "TestFoo"}, []string{"X_TestFoo", "F_TestFoo", "TestFoo"}},
		{"suite rejects unexported or embedded TestSuite", gotestast.IS_TEST_SUITE, []string{"TESTSUITE", "TestSuite", "_TestSuite", "ABCTestSuiteABC"}, nil},
		{"suite rejects generated harness names", gotestast.IS_TEST_SUITE, []string{"ƒƒ_GOTEST_ABCTestSuite", "ƒƒ_GOTEST_F_ABCTestSuite"}, nil},
		{"suite matches exported TestSuite types", gotestast.IS_TEST_SUITE, []string{"X_FooTestSuite", "F_FooTestSuite", "FooTestSuite", "Foo_TestSuite"}, []string{
			"X_FooTestSuite", "F_FooTestSuite", "FooTestSuite", "Foo_TestSuite"}},
	}

	for _, tC := range testCases {
		t.When(tC.desc, func(w *gotest.T) {
			w.It("matches correctly", func(it *gotest.T) {
				var actualMatches []string
				for _, v := range tC.in {
					ok := tC.fn.MatchString(v)
					if ok {
						actualMatches = append(actualMatches, v)
					}
				}
				gotest.Equal(it, tC.out, actualMatches)
			})
		})
	}
}
