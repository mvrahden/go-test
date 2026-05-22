package gotestgen //nolint:stdlib-test

import (
	"fmt"
	"go/token"
	"go/types"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/mvrahden/go-test/internal/gotestast"
	"github.com/mvrahden/go-test/pkg/gotest"
	"golang.org/x/tools/go/packages"
)

var sharedModRoot struct {
	once sync.Once
	dir  string
	err  error
}

func initSharedModRoot() {
	modRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		sharedModRoot.err = err
		return
	}

	// Bootstrap: run go mod tidy in a scratch dir to produce go.sum.
	scratch, err := os.MkdirTemp("", "gotest-mod-init-*")
	if err != nil {
		sharedModRoot.err = err
		return
	}
	defer os.RemoveAll(scratch)

	goMod := []byte("module testpkg\n\ngo 1.24\n\nrequire github.com/mvrahden/go-test v0.0.0\n\nreplace github.com/mvrahden/go-test => " + modRoot + "\n")
	if err := os.WriteFile(filepath.Join(scratch, "go.mod"), goMod, 0644); err != nil {
		sharedModRoot.err = err
		return
	}
	if err := os.WriteFile(filepath.Join(scratch, "stub.go"), []byte("package testpkg\n\nimport _ \"github.com/mvrahden/go-test/pkg/gotest\"\n"), 0644); err != nil {
		sharedModRoot.err = err
		return
	}

	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = scratch
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		sharedModRoot.err = fmt.Errorf("go mod tidy: %w\n%s", err, out)
		return
	}

	// Persistent shared module root — all tests create sub-packages here.
	dir, err := os.MkdirTemp("", "gotest-shared-root-*")
	if err != nil {
		sharedModRoot.err = err
		return
	}

	tidiedMod, _ := os.ReadFile(filepath.Join(scratch, "go.mod"))
	tidiedSum, _ := os.ReadFile(filepath.Join(scratch, "go.sum"))
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), tidiedMod, 0644); err != nil {
		sharedModRoot.err = err
		return
	}
	if len(tidiedSum) > 0 {
		if err := os.WriteFile(filepath.Join(dir, "go.sum"), tidiedSum, 0644); err != nil {
			sharedModRoot.err = err
			return
		}
	}
	sharedModRoot.dir = dir
}

func TestMain(m *testing.M) {
	code := m.Run()
	if sharedModRoot.dir != "" {
		os.RemoveAll(sharedModRoot.dir)
	}
	os.Exit(code)
}

func loadTestPkgWithGotest(t *testing.T, src string) *packages.Package {
	t.Helper()

	sharedModRoot.once.Do(initSharedModRoot)
	if sharedModRoot.err != nil {
		t.Fatal(sharedModRoot.err)
	}

	subDir := filepath.Join(sharedModRoot.dir, t.Name())
	gotest.NoError(t, os.MkdirAll(subDir, 0755))
	gotest.NoError(t, os.WriteFile(filepath.Join(subDir, "test.go"), []byte(src), 0644))

	pkgs, err := packages.Load(&packages.Config{
		Mode: packageEvalMode,
		Dir:  subDir,
		Env:  append(os.Environ(), "GOWORK=off"),
	}, ".")
	gotest.NoError(t, err)
	gotest.True(t, len(pkgs) > 0, "expected at least one package loaded")

	pkg := pkgs[0]
	gotest.True(t, len(pkg.Errors) == 0, "expected no package errors, got: %v", pkg.Errors)
	return pkg
}

// --- Fixture collection tests ---

func TestCollector_FixtureCollection_PackageFixture(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import "context"

type DBFixture struct {
	Conn string
}

func (f *DBFixture) BeforeAll(ctx context.Context) error { return nil }
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))
	gotest.Equal(t, 1, len(result.Fixtures))
	gotest.Equal(t, gotestast.PackageFixture, result.Fixtures[0].Kind)
	gotest.Equal(t, "DBFixture", result.Fixtures[0].Identifier())
	gotest.True(t, result.Fixtures[0].BeforeAll != nil, "expected BeforeAll to be set")
	gotest.True(t, result.Fixtures[0].AfterAll == nil, "expected AfterAll to be nil")
}

func TestCollector_FixtureCollection_PackageFixtureAllMethods(t *testing.T) {
	t.Parallel()
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
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))
	gotest.Equal(t, 1, len(result.Fixtures))

	fix := result.Fixtures[0]
	gotest.True(t, fix.BeforeAll != nil, "expected BeforeAll")
	gotest.True(t, fix.AfterAll != nil, "expected AfterAll")
	gotest.True(t, fix.BeforeEach != nil, "expected BeforeEach")
	gotest.True(t, fix.AfterEach != nil, "expected AfterEach")
}

func TestCollector_FixtureCollection_SharedFixture(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import "context"

type RedisSharedFixture struct {
	Addr string
}

func (f *RedisSharedFixture) BeforeAll(ctx context.Context) error { return nil }
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))
	gotest.Equal(t, 1, len(result.Fixtures))
	gotest.Equal(t, gotestast.SharedFixture, result.Fixtures[0].Kind)
	gotest.True(t, result.Fixtures[0].BeforeAll != nil, "expected BeforeAll to be set")
}

func TestCollector_FixtureCollection_SharedFixtureWithAfterAll(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import "context"

type RedisSharedFixture struct {
	Addr string
}

func (f *RedisSharedFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *RedisSharedFixture) AfterAll(ctx context.Context) error  { return nil }
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))
	gotest.Equal(t, 1, len(result.Fixtures))

	fix := result.Fixtures[0]
	gotest.True(t, fix.BeforeAll != nil, "expected BeforeAll")
	gotest.True(t, fix.AfterAll != nil, "expected AfterAll")
}

// --- Fixture embedding in test suites ---

func TestCollector_FixtureEmbeddingInTestSuite(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type DBFixture struct {
	Conn string
}

func (f *DBFixture) BeforeAll(ctx context.Context) error { return nil }

type MyTestSuite struct {
	*DBFixture
}

func (s *MyTestSuite) TestSomething(t *gotest.T) {}
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))
	gotest.Equal(t, 1, len(result.Suites))
	gotest.Equal(t, 1, len(result.Fixtures))
	gotest.Equal(t, "DBFixture", result.Fixtures[0].Identifier())
}

func TestCollector_NoFixtureEmbedding(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type MyTestSuite struct{}

func (s *MyTestSuite) TestSomething(t *gotest.T) {}
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))
	gotest.Equal(t, 1, len(result.Suites))
	gotest.True(t, result.Suites[0].Fixture() == nil, "expected no fixture")
}

// --- Fixture-to-fixture embedding ---

func TestCollector_FixtureToFixtureEmbedding(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import "context"

type BaseFixture struct{}

func (f *BaseFixture) BeforeAll(ctx context.Context) error { return nil }

type DBFixture struct {
	*BaseFixture
}

func (f *DBFixture) BeforeAll(ctx context.Context) error { return nil }
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	// Note: this will error because there are 2 package fixtures
	// For this test we just verify collection worked up to that point
	gotest.Equal(t, 0, len(result.Errs))
	gotest.Equal(t, 2, len(result.Fixtures))
}


// --- Validation: shared fixture wrong signature ---

func TestCollector_SharedFixture_BeforeEachDisallowed(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import "context"

type RedisSharedFixture struct{}

func (f *RedisSharedFixture) BeforeAll(ctx context.Context) error    { return nil }
func (f *RedisSharedFixture) BeforeEach(ctx context.Context) error   { return nil }
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.True(t, len(result.Errs) > 0, "expected error for BeforeEach on shared fixture")
	gotest.Contains(t, result.Errs[0].Err.Error(), "must not have BeforeEach")
}

func TestCollector_SharedFixture_AfterEachDisallowed(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import "context"

type RedisSharedFixture struct{}

func (f *RedisSharedFixture) BeforeAll(ctx context.Context) error  { return nil }
func (f *RedisSharedFixture) AfterEach(ctx context.Context) error  { return nil }
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.True(t, len(result.Errs) > 0, "expected error for AfterEach on shared fixture")
	gotest.Contains(t, result.Errs[0].Err.Error(), "must not have AfterEach")
}

func TestCollector_SharedFixture_WrongBeforeAllSignature(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type RedisSharedFixture struct{}

func (f *RedisSharedFixture) BeforeAll(t *gotest.T) {} // wrong: should be (ctx context.Context) error
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.True(t, len(result.Errs) > 0, "expected error for wrong BeforeAll signature on shared fixture")
	gotest.Contains(t, result.Errs[0].Err.Error(), "unsupported signature")
}

// --- Validation: ApplyTestSuiteSpecs ---

func TestApplyTestSuiteSpecs_OK(t *testing.T) {
	c := collector{}
	spec, err := c.ApplyTestSuiteSpecs(CollectorResult{
		Fixtures: []*gotestast.FixtureSpec{
			makeFixtureSpec("Fix1", gotestast.PackageFixture, true),
		},
	})
	gotest.NoError(t, err)
	gotest.True(t, spec.EffectiveTestSuites == nil, "expected no suites")
}

// --- Package fixture wrong method signature ---

func TestCollector_PackageFixture_WrongBeforeAllSignature(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type DBFixture struct{}

func (f *DBFixture) BeforeAll(t *gotest.T) {} // wrong: should be (ctx context.Context) error
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.True(t, len(result.Errs) > 0, "expected error for wrong BeforeAll signature on package fixture")
	gotest.Contains(t, result.Errs[0].Err.Error(), "unsupported signature")
}

// --- *testing.T suite support ---

func TestCollector_StdlibT_SuiteDetected(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import "testing"

type PlainTestSuite struct{}

func (s *PlainTestSuite) TestFoo(t *testing.T) {}
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))
	gotest.Equal(t, 1, len(result.Suites))
	gotest.Equal(t, "PlainTestSuite", result.Suites[0].Identifier())
	gotest.Equal(t, 1, len(result.Suites[0].TestCases()))
	gotest.True(t, result.Suites[0].TestCases()[0].UsesStdlibT(), "expected UsesStdlibT for *testing.T method")
}

func TestCollector_StdlibT_LifecycleHooks(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import "testing"

type HookTestSuite struct{}

func (s *HookTestSuite) BeforeAll(t *testing.T)  {}
func (s *HookTestSuite) AfterAll(t *testing.T)   {}
func (s *HookTestSuite) BeforeEach(t *testing.T) {}
func (s *HookTestSuite) AfterEach(t *testing.T)  {}
func (s *HookTestSuite) TestOne(t *testing.T)    {}
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))
	gotest.Equal(t, 1, len(result.Suites))

	s := result.Suites[0]
	gotest.True(t, s.BeforeAll() != nil, "expected BeforeAll")
	gotest.True(t, s.BeforeAll().UsesStdlibT(), "expected BeforeAll UsesStdlibT")
	gotest.True(t, s.AfterAll() != nil, "expected AfterAll")
	gotest.True(t, s.AfterAll().UsesStdlibT(), "expected AfterAll UsesStdlibT")
	gotest.True(t, s.BeforeEach() != nil, "expected BeforeEach")
	gotest.True(t, s.BeforeEach().UsesStdlibT(), "expected BeforeEach UsesStdlibT")
	gotest.True(t, s.AfterEach() != nil, "expected AfterEach")
	gotest.True(t, s.AfterEach().UsesStdlibT(), "expected AfterEach UsesStdlibT")
}

func TestCollector_StdlibT_MixedMethodSignatures(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import (
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type MixedTestSuite struct{}

func (s *MixedTestSuite) TestStdlib(t *testing.T) {}
func (s *MixedTestSuite) TestGotest(t *gotest.T)  {}
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))
	gotest.Equal(t, 1, len(result.Suites))
	gotest.Equal(t, 2, len(result.Suites[0].TestCases()))

	cases := result.Suites[0].TestCases()
	gotest.Equal(t, "TestStdlib", cases[0].Identifier())
	gotest.True(t, cases[0].UsesStdlibT(), "expected TestStdlib UsesStdlibT")
	gotest.Equal(t, "TestGotest", cases[1].Identifier())
	gotest.True(t, !cases[1].UsesStdlibT(), "expected TestGotest NOT UsesStdlibT")
}

func TestCollector_StdlibT_WrongParamType(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import "fmt"

type BadTestSuite struct{}

func (s *BadTestSuite) TestBad(f fmt.Stringer) {}
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.True(t, len(result.Errs) > 0, "expected error for unsupported param type")
	gotest.Contains(t, result.Errs[0].Err.Error(), "must be *gotest.T or *testing.T")
}

func TestCollector_GotestT_NotUsesStdlibT(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type GotestTestSuite struct{}

func (s *GotestTestSuite) TestFoo(t *gotest.T) {}
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))
	gotest.Equal(t, 1, len(result.Suites[0].TestCases()))
	gotest.True(t, !result.Suites[0].TestCases()[0].UsesStdlibT(), "expected NOT UsesStdlibT for *gotest.T")
}

// --- Nil package ---

func TestCollector_NilPackage(t *testing.T) {
	c := collector{}
	result := c.CollectSuiteSpecs(nil)
	gotest.Equal(t, 0, len(result.Errs))
	gotest.True(t, result.Suites == nil, "expected nil suites")
	gotest.True(t, result.Fixtures == nil, "expected nil fixtures")
}

// --- Shared fixture embedding: not treated as parent ---

func TestCollector_SharedFixtureNotTreatedAsParent(t *testing.T) {
	t.Parallel()
	src := "package testpkg\n\n" +
		"import (\n" +
		"\t\"context\"\n\n" +
		"\t\"github.com/mvrahden/go-test/pkg/gotest\"\n" +
		")\n\n" +
		"type PGSharedFixture struct {\n" +
		"\tDSN string `gotest:\"env=PG_DSN\"`\n" +
		"}\n\n" +
		"func (f *PGSharedFixture) BeforeAll(ctx context.Context) error { return nil }\n\n" +
		"type E2EFixture struct {\n" +
		"\t*PGSharedFixture\n" +
		"}\n\n" +
		"func (f *E2EFixture) BeforeAll(ctx context.Context) error { return nil }\n\n" +
		"type QueryTestSuite struct {\n" +
		"\t*E2EFixture\n" +
		"}\n\n" +
		"func (s *QueryTestSuite) TestInsert(t *gotest.T) {}\n"

	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs), "expected no errors, got: %v", result.Errs)
	gotest.Equal(t, 1, len(result.Suites))
	gotest.Equal(t, 2, len(result.Fixtures))

	// Verify both fixtures were collected
	names := map[string]bool{}
	for _, f := range result.Fixtures {
		names[f.Identifier()] = true
	}
	gotest.True(t, names["E2EFixture"], "expected E2EFixture")
	gotest.True(t, names["PGSharedFixture"], "expected PGSharedFixture")
}

// --- Config marker method tests ---

func TestCollector_FixtureConfig_Detected(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type DBFixture struct{}

func (f *DBFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *DBFixture) FixtureConfig() gotest.FixtureConfig {
	return gotest.DefaultFixtureConfig()
}
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))
	gotest.Equal(t, 1, len(result.Fixtures))
	gotest.True(t, result.Fixtures[0].Config != nil, "expected Config to be set")
}

func TestCollector_SharedFixtureConfig_Detected(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type PGSharedFixture struct{}

func (f *PGSharedFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *PGSharedFixture) SharedFixtureConfig() gotest.FixtureConfig {
	return gotest.ContainerFixtureConfig()
}
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs), "expected no errors, got: %v", result.Errs)
	gotest.Equal(t, 1, len(result.Fixtures))
	gotest.True(t, result.Fixtures[0].Config != nil, "expected Config to be set via SharedFixtureConfig")
}

func TestCollector_FixtureConfig_AbsentIsNil(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import "context"

type PlainFixture struct{}

func (f *PlainFixture) BeforeAll(ctx context.Context) error { return nil }
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))
	gotest.Equal(t, 1, len(result.Fixtures))
	gotest.True(t, result.Fixtures[0].Config == nil, "expected Config to be nil")
}

func TestCollector_SuiteConfig_Detected(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type MyTestSuite struct{}

func (s *MyTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{}
}
func (s *MyTestSuite) TestFoo(t *gotest.T) {}
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))
	gotest.Equal(t, 1, len(result.Suites))
	gotest.True(t, result.Suites[0].HasConfig(), "expected HasConfig() to be true")
}

func TestCollector_SuiteConfig_AbsentIsFalse(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type PlainTestSuite struct{}

func (s *PlainTestSuite) TestFoo(t *gotest.T) {}
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))
	gotest.Equal(t, 1, len(result.Suites))
	gotest.True(t, !result.Suites[0].HasConfig(), "expected HasConfig() to be false")
}

func TestCollector_FixtureConfig_InvalidSignature_WithParams(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type BadFixture struct{}

func (f *BadFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *BadFixture) FixtureConfig(x int) gotest.FixtureConfig {
	return gotest.DefaultFixtureConfig()
}
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.NotEmpty(t, result.Errs, "expected error for invalid FixtureConfig signature")
	gotest.Contains(t, result.Errs[0].Err.Error(), "unsupported signature")
}

func TestCollector_FixtureConfig_InvalidSignature_WrongReturnType(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import "context"

type BadFixture struct{}

func (f *BadFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *BadFixture) FixtureConfig() string { return "" }
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.NotEmpty(t, result.Errs, "expected error for wrong FixtureConfig return type")
	gotest.Contains(t, result.Errs[0].Err.Error(), "unsupported return type")
}

func TestCollector_SuiteConfig_InvalidSignature_WithParams(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type BadTestSuite struct{}

func (s *BadTestSuite) SuiteConfig(x int) gotest.SuiteConfig {
	return gotest.DefaultSuiteConfig()
}
func (s *BadTestSuite) TestFoo(t *gotest.T) {}
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.NotEmpty(t, result.Errs, "expected error for invalid SuiteConfig signature")
	gotest.Contains(t, result.Errs[0].Err.Error(), "unsupported signature")
}

func TestCollector_SuiteConfig_InvalidSignature_WrongReturnType(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type BadTestSuite struct{}

func (s *BadTestSuite) SuiteConfig() int { return 0 }
func (s *BadTestSuite) TestFoo(t *gotest.T) {}
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.NotEmpty(t, result.Errs, "expected error for wrong SuiteConfig return type")
	gotest.Contains(t, result.Errs[0].Err.Error(), "unsupported return type")
}

func TestCollector_SuiteGuard_Detected(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type MyTestSuite struct{}

func (s *MyTestSuite) SuiteGuard() string { return "" }
func (s *MyTestSuite) TestFoo(t *gotest.T) {}
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))
	gotest.Equal(t, 1, len(result.Suites))
	gotest.True(t, result.Suites[0].HasGuard(), "expected HasGuard() to be true")
}

func TestCollector_SuiteGuard_AbsentIsFalse(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type PlainTestSuite struct{}

func (s *PlainTestSuite) TestFoo(t *gotest.T) {}
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))
	gotest.Equal(t, 1, len(result.Suites))
	gotest.True(t, !result.Suites[0].HasGuard(), "expected HasGuard() to be false")
}

func TestCollector_SuiteGuard_InvalidSignature_WithParams(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type BadTestSuite struct{}

func (s *BadTestSuite) SuiteGuard(x int) string { return "" }
func (s *BadTestSuite) TestFoo(t *gotest.T) {}
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.NotEmpty(t, result.Errs, "expected error for invalid SuiteGuard signature")
	gotest.Contains(t, result.Errs[0].Err.Error(), "unsupported signature")
}

func TestCollector_SuiteGuard_InvalidSignature_WrongReturnType(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type BadTestSuite struct{}

func (s *BadTestSuite) SuiteGuard() int { return 0 }
func (s *BadTestSuite) TestFoo(t *gotest.T) {}
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.NotEmpty(t, result.Errs, "expected error for wrong SuiteGuard return type")
	gotest.Contains(t, result.Errs[0].Err.Error(), "unsupported return type")
}

// --- Context param and return type tests ---

func TestCollector_BeforeEach_ReturningForm(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type myCtx struct{ val string }

type MyTestSuite struct{}

func (s *MyTestSuite) BeforeEach(t *gotest.T) *myCtx { return &myCtx{} }
func (s *MyTestSuite) TestOne(t *gotest.T, ctx *myCtx) {}
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs), "expected no errors, got: %v", result.Errs)
	gotest.Equal(t, 1, len(result.Suites))

	be := result.Suites[0].BeforeEach()
	gotest.True(t, be != nil, "expected BeforeEach")
	gotest.True(t, be.HasReturn(), "expected BeforeEach to have return type")
}

func TestCollector_BeforeEach_TooManyReturns(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type myCtx struct{}

type BadTestSuite struct{}

func (s *BadTestSuite) BeforeEach(t *gotest.T) (*myCtx, error) { return nil, nil }
func (s *BadTestSuite) TestOne(t *gotest.T) {}
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.NotEmpty(t, result.Errs, "expected error for 2 return values")
	gotest.Contains(t, result.Errs[0].Err.Error(), "expected 0 or 1 return values")
}

func TestCollector_AfterEach_WithContextParam(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type myCtx struct{ val string }

type MyTestSuite struct{}

func (s *MyTestSuite) BeforeEach(t *gotest.T) *myCtx { return &myCtx{} }
func (s *MyTestSuite) AfterEach(t *gotest.T, ctx *myCtx) {}
func (s *MyTestSuite) TestOne(t *gotest.T, ctx *myCtx) {}
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs), "expected no errors, got: %v", result.Errs)

	ae := result.Suites[0].AfterEach()
	gotest.True(t, ae != nil, "expected AfterEach")
	gotest.True(t, ae.HasContextParam(), "expected AfterEach to have context param")
}

func TestCollector_AfterEach_TooManyParams(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type myCtx struct{}

type BadTestSuite struct{}

func (s *BadTestSuite) AfterEach(t *gotest.T, ctx *myCtx, extra int) {}
func (s *BadTestSuite) TestOne(t *gotest.T) {}
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.NotEmpty(t, result.Errs, "expected error for 3 params")
}

func TestCollector_TestMethod_WithContextParam(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type myCtx struct{}

type MyTestSuite struct{}

func (s *MyTestSuite) BeforeEach(t *gotest.T) *myCtx { return &myCtx{} }
func (s *MyTestSuite) TestOne(t *gotest.T, ctx *myCtx) {}
func (s *MyTestSuite) TestTwo(t *gotest.T, _ *myCtx) {}
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs), "expected no errors, got: %v", result.Errs)
	gotest.Equal(t, 2, len(result.Suites[0].TestCases()))
	gotest.True(t, result.Suites[0].TestCases()[0].HasContextParam(), "expected TestOne to have context param")
	gotest.True(t, result.Suites[0].TestCases()[1].HasContextParam(), "expected TestTwo to have context param")
}

func TestCollector_TestMethod_AsyncWithContext(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type myCtx struct{}

type MyTestSuite struct{}

func (s *MyTestSuite) BeforeEach(t *gotest.T) *myCtx { return &myCtx{} }
func (s *MyTestSuite) TestOneAsync(t *gotest.T, ctx *myCtx, done func()) {}
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs), "expected no errors, got: %v", result.Errs)
	gotest.Equal(t, 1, len(result.Suites[0].TestCases()))
	gotest.True(t, result.Suites[0].TestCases()[0].HasContextParam(), "expected context param")
}

func TestCollector_SuiteConfig_ParallelParsed(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type myCtx struct{}

type MyTestSuite struct{}

func (s *MyTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}
func (s *MyTestSuite) BeforeEach(t *gotest.T) *myCtx { return &myCtx{} }
func (s *MyTestSuite) TestOne(t *gotest.T, ctx *myCtx) {}
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs), "expected no errors, got: %v", result.Errs)
	gotest.True(t, result.Suites[0].IsMethodParallel(), "expected IsMethodParallel to be true")
}

func TestCollector_SuiteConfig_NonLiteralBody_Error(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type MyTestSuite struct{}

var cfg = gotest.SuiteConfig{Parallel: true}

func (s *MyTestSuite) SuiteConfig() gotest.SuiteConfig {
	return cfg
}
func (s *MyTestSuite) TestOne(t *gotest.T) {}
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.NotEmpty(t, result.Errs, "expected error for non-literal SuiteConfig body")
}

func TestCollector_Validation_ParallelRequiresReturningBeforeEach(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type MyTestSuite struct{}

func (s *MyTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}
func (s *MyTestSuite) BeforeEach(t *gotest.T) {}
func (s *MyTestSuite) TestOne(t *gotest.T) {}
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.NotEmpty(t, result.Errs, "expected error: parallel requires returning BeforeEach")
	gotest.Contains(t, result.Errs[0].Err.Error(), "Parallel")
}

func TestCollector_Validation_ParallelWithoutBeforeEach_Allowed(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type MyTestSuite struct{}

func (s *MyTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}
func (s *MyTestSuite) TestOne(t *gotest.T) {}
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs), "parallel with no BeforeEach should be allowed")
}

func TestCollector_Validation_MethodMissingContextParam(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type myCtx struct{}

type MyTestSuite struct{}

func (s *MyTestSuite) BeforeEach(t *gotest.T) *myCtx { return &myCtx{} }
func (s *MyTestSuite) TestOne(t *gotest.T, ctx *myCtx) {}
func (s *MyTestSuite) TestTwo(t *gotest.T) {}
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.NotEmpty(t, result.Errs, "expected error: TestTwo missing context param")
	gotest.Contains(t, result.Errs[0].Err.Error(), "TestTwo")
}

func TestCollector_Validation_AfterEachMissingContextParam(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type myCtx struct{}

type MyTestSuite struct{}

func (s *MyTestSuite) BeforeEach(t *gotest.T) *myCtx { return &myCtx{} }
func (s *MyTestSuite) AfterEach(t *gotest.T) {}
func (s *MyTestSuite) TestOne(t *gotest.T, ctx *myCtx) {}
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.NotEmpty(t, result.Errs, "expected error: AfterEach missing context param")
	gotest.Contains(t, result.Errs[0].Err.Error(), "AfterEach")
}

func TestCollector_Validation_OrphanContextAfterEach(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type myCtx struct{}

type MyTestSuite struct{}

func (s *MyTestSuite) AfterEach(t *gotest.T, ctx *myCtx) {}
func (s *MyTestSuite) TestOne(t *gotest.T) {}
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.NotEmpty(t, result.Errs, "expected error: orphan context AfterEach")
}

func TestCollector_Validation_TypeMismatch(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type myCtx struct{}
type otherCtx struct{}

type MyTestSuite struct{}

func (s *MyTestSuite) BeforeEach(t *gotest.T) *myCtx { return &myCtx{} }
func (s *MyTestSuite) TestOne(t *gotest.T, ctx *otherCtx) {}
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.NotEmpty(t, result.Errs, "expected error: type mismatch")
	gotest.Contains(t, result.Errs[0].Err.Error(), "does not match")
}

func TestCollector_Validation_ReturningBeforeEach_FullyConsistent_OK(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type myCtx struct{}

type MyTestSuite struct{}

func (s *MyTestSuite) BeforeEach(t *gotest.T) *myCtx { return &myCtx{} }
func (s *MyTestSuite) AfterEach(t *gotest.T, ctx *myCtx) {}
func (s *MyTestSuite) TestOne(t *gotest.T, ctx *myCtx) {}
func (s *MyTestSuite) TestTwo(t *gotest.T, _ *myCtx) {}
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs), "expected no errors, got: %v", result.Errs)
}

// --- helpers ---

// makeFixtureSpec creates a minimal FixtureSpec for validation testing.
// It uses gotestast.NewFixtureSpecForTest which we need to add.
func makeFixtureSpec(name string, kind gotestast.FixtureKind, hasBeforeAll bool) *gotestast.FixtureSpec {
	f := gotestast.NewFixtureSpecForTest(name, kind)
	if hasBeforeAll {
		f.BeforeAll = types.NewFunc(token.NoPos, nil, "BeforeAll", nil)
	}
	return f
}
