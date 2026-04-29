package gotestgen

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/mvrahden/go-test/internal/gotestast"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// TestSharedFixture_Integration_GeneratedSetupBinary verifies that
// GenerateSharedSetup produces a valid, correctly structured Go program
// with context-aware lifecycle calls and field-level JSON serialization.
func TestSharedFixture_Integration_GeneratedSetupBinary(t *testing.T) {
	fixtures := []SharedFixtureInfo{
		{
			Identifier:     "PostgresFixture",
			PkgPath:        "github.com/example/project/tests/fixtures",
			TransferFields: []string{"DSN"},
		},
		{
			Identifier:     "GCSFixture",
			PkgPath:        "github.com/example/project/tests/gcs",
			TransferFields: []string{"Endpoint", "Bucket"},
		},
	}

	src, err := GenerateSharedSetup(fixtures)
	gotest.NoError(t, err)

	code := string(src)

	fset := token.NewFileSet()
	_, parseErr := parser.ParseFile(fset, "setup.go", code, parser.AllErrors)
	gotest.NoError(t, parseErr, "generated code should be valid Go: %s", code)

	gotest.Contains(t, code, "package main")
	gotest.Contains(t, code, `"context"`)
	gotest.Contains(t, code, `"github.com/example/project/tests/fixtures"`)
	gotest.Contains(t, code, `"github.com/example/project/tests/gcs"`)

	gotest.Contains(t, code, "PostgresFixture{}")
	gotest.Contains(t, code, "GCSFixture{}")

	gotest.Contains(t, code, "ctx := context.Background()")
	gotest.Contains(t, code, "sf0.BeforeAll(ctx)")
	gotest.Contains(t, code, "sf1.BeforeAll(ctx)")

	gotest.Contains(t, code, "syscall.SIGTERM")
	gotest.Contains(t, code, "json.NewEncoder(os.Stdout).Encode(state)")

	gotest.True(t, strings.Contains(code, "sf0.DSN"), "DSN should be serialized")
	gotest.True(t, strings.Contains(code, "sf1.Endpoint"), "Endpoint should be serialized")
	gotest.True(t, strings.Contains(code, "sf1.Bucket"), "Bucket should be serialized")

	gcsAfterAllIdx := strings.LastIndex(code, "sf1.AfterAll(ctx)")
	pgAfterAllIdx := strings.LastIndex(code, "sf0.AfterAll(ctx)")
	gotest.True(t, gcsAfterAllIdx < pgAfterAllIdx,
		"expected reverse teardown order: sf1 (GCSFixture) before sf0 (PostgresFixture)")
}

// TestSharedFixture_Integration_DiscoverFromRealPackage verifies that
// DiscoverSharedFixtures correctly finds shared fixtures collected from
// a real package loaded via packages.Load.
func TestSharedFixture_Integration_DiscoverFromRealPackage(t *testing.T) {
	src := `package testpkg

import "context"

type PostgresSharedFixture struct {
	DSN string
}

func (f *PostgresSharedFixture) BeforeAll(ctx context.Context) error { return nil }
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)

	gotest.Equal(t, 0, len(result.Errs), "expected no collection errors")
	gotest.Equal(t, 1, len(result.Fixtures), "expected one fixture")
	gotest.Equal(t, gotestast.SharedFixture, result.Fixtures[0].Kind)

	shared := DiscoverSharedFixtures([]CollectorResult{result})

	gotest.Equal(t, 1, len(shared), "expected one shared fixture")
	gotest.Equal(t, "PostgresSharedFixture", shared[0].Identifier)
	gotest.Equal(t, 1, len(shared[0].TransferFields), "DSN should be transferable")
	gotest.Equal(t, "DSN", shared[0].TransferFields[0])
}

// TestSharedFixture_Integration_DiscoverFromRealPackage_MultipleFixtures verifies
// that DiscoverSharedFixtures collects multiple shared fixtures from multiple
// collector results and deduplicates correctly.
func TestSharedFixture_Integration_DiscoverFromRealPackage_MultipleFixtures(t *testing.T) {
	src1 := `package testpkg

import "context"

type PostgresSharedFixture struct {
	DSN string
}

func (f *PostgresSharedFixture) BeforeAll(ctx context.Context) error { return nil }
`
	src2 := `package testpkg

import "context"

type RedisSharedFixture struct {
	Addr string
}

func (f *RedisSharedFixture) BeforeAll(ctx context.Context) error { return nil }
`
	pkg1 := loadTestPkgWithGotest(t, src1)
	pkg2 := loadTestPkgWithGotest(t, src2)

	c := collector{}
	result1 := c.CollectSuiteSpecs(pkg1)
	result2 := c.CollectSuiteSpecs(pkg2)

	gotest.Equal(t, 0, len(result1.Errs))
	gotest.Equal(t, 0, len(result2.Errs))

	shared := DiscoverSharedFixtures([]CollectorResult{result1, result2})

	gotest.Equal(t, 2, len(shared), "expected two shared fixtures")

	found := map[string]SharedFixtureInfo{}
	for _, sf := range shared {
		found[sf.Identifier] = sf
	}

	pg, ok := found["PostgresSharedFixture"]
	gotest.True(t, ok, "expected PostgresSharedFixture in discovered fixtures")
	gotest.Equal(t, 1, len(pg.TransferFields))
	gotest.Equal(t, "DSN", pg.TransferFields[0])

	redis, ok := found["RedisSharedFixture"]
	gotest.True(t, ok, "expected RedisSharedFixture in discovered fixtures")
	gotest.Equal(t, 1, len(redis.TransferFields))
	gotest.Equal(t, "Addr", redis.TransferFields[0])
}

func TestSharedFixture_Integration_DiscoverWithHydrate(t *testing.T) {
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
func (f *PGSharedFixture) Dehydrate(ctx context.Context) error { return nil }
func (f *PGSharedFixture) connect(ctx context.Context) error {
	f.Pool = 42
	return nil
}
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs), "expected no errors, got: %v", result.Errs)
	gotest.Equal(t, 1, len(result.Fixtures))

	shared := DiscoverSharedFixtures([]CollectorResult{result})
	gotest.Equal(t, 1, len(shared))

	sf := shared[0]
	gotest.Equal(t, "PGSharedFixture", sf.Identifier)
	gotest.True(t, sf.HasHydrate, "HasHydrate should be true")
	gotest.True(t, sf.HasDehydrate, "HasDehydrate should be true")

	gotest.Equal(t, 2, len(sf.TransferFields), "ConnStr and Port should be transferable")
	gotest.Equal(t, "ConnStr", sf.TransferFields[0])
	gotest.Equal(t, "Port", sf.TransferFields[1])

	gotest.Equal(t, 1, len(sf.LocalFields), "Pool should be local (assigned in connect helper)")
	gotest.Equal(t, "Pool", sf.LocalFields[0])
}
