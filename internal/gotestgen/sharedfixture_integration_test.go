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
// when given multiple fixtures with env tags.
func TestSharedFixture_Integration_GeneratedSetupBinary(t *testing.T) {
	// Given two shared fixtures with env tags
	fixtures := []SharedFixtureInfo{
		{
			Identifier: "PostgresFixture",
			PkgPath:    "github.com/example/project/tests/fixtures",
			EnvTags:    map[string]string{"DSN": "E2E_POSTGRES_DSN"},
		},
		{
			Identifier: "GCSFixture",
			PkgPath:    "github.com/example/project/tests/gcs",
			EnvTags: map[string]string{
				"Endpoint": "E2E_GCS_ENDPOINT",
				"Bucket":   "E2E_GCS_BUCKET",
			},
		},
	}

	// When generating the setup binary
	src, err := GenerateSharedSetup(fixtures)
	gotest.NoError(t, err)

	code := string(src)

	// Verify it is valid Go source
	fset := token.NewFileSet()
	_, parseErr := parser.ParseFile(fset, "setup.go", code, parser.AllErrors)
	gotest.NoError(t, parseErr, "generated code should be valid Go: %s", code)

	// Verify it has package main
	gotest.Contains(t, code, "package main")

	// Verify correct imports
	gotest.Contains(t, code, `"github.com/example/project/tests/fixtures"`)
	gotest.Contains(t, code, `"github.com/example/project/tests/gcs"`)

	// Verify correct fixture initialization
	gotest.Contains(t, code, "PostgresFixture{}")
	gotest.Contains(t, code, "GCSFixture{}")

	// Verify correct env var mapping
	gotest.Contains(t, code, `"E2E_POSTGRES_DSN"`)
	gotest.Contains(t, code, `"E2E_GCS_ENDPOINT"`)
	gotest.Contains(t, code, `"E2E_GCS_BUCKET"`)

	// Verify proper signal handling
	gotest.Contains(t, code, "syscall.SIGTERM")
	gotest.Contains(t, code, "json.NewEncoder(os.Stdout)")

	// Verify BeforeAll calls are present
	gotest.Contains(t, code, "sf0.BeforeAll()")
	gotest.Contains(t, code, "sf1.BeforeAll()")

	// Verify reverse-order teardown:
	// sf1.AfterAll() (GCSFixture) should appear before sf0.AfterAll() (PostgresFixture)
	// in the final teardown block at the end of main.
	gcsAfterAllIdx := strings.LastIndex(code, "sf1.AfterAll()")
	pgAfterAllIdx := strings.LastIndex(code, "sf0.AfterAll()")
	gotest.True(t, gcsAfterAllIdx < pgAfterAllIdx,
		"expected reverse teardown order: sf1 (GCSFixture) before sf0 (PostgresFixture)")
}

// TestSharedFixture_Integration_DiscoverFromRealPackage verifies that
// DiscoverSharedFixtures correctly finds shared fixtures collected from
// a real package loaded via packages.Load.
func TestSharedFixture_Integration_DiscoverFromRealPackage(t *testing.T) {
	src := `package testpkg

type PostgresSharedFixture struct {
	DSN string ` + "`" + `gotest:"env=E2E_POSTGRES_DSN"` + "`" + `
}

func (f *PostgresSharedFixture) BeforeAll() error { return nil }
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
	gotest.Equal(t, "E2E_POSTGRES_DSN", shared[0].EnvTags["DSN"],
		"env tag for DSN field should be E2E_POSTGRES_DSN")
}

// TestSharedFixture_Integration_DiscoverFromRealPackage_MultipleFixtures verifies
// that DiscoverSharedFixtures collects multiple shared fixtures from multiple
// collector results and deduplicates correctly.
func TestSharedFixture_Integration_DiscoverFromRealPackage_MultipleFixtures(t *testing.T) {
	src1 := `package testpkg

type PostgresSharedFixture struct {
	DSN string ` + "`" + `gotest:"env=E2E_POSTGRES_DSN"` + "`" + `
}

func (f *PostgresSharedFixture) BeforeAll() error { return nil }
`
	src2 := `package testpkg

type RedisSharedFixture struct {
	Addr string ` + "`" + `gotest:"env=E2E_REDIS_ADDR"` + "`" + `
}

func (f *RedisSharedFixture) BeforeAll() error { return nil }
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
	gotest.Equal(t, "E2E_POSTGRES_DSN", pg.EnvTags["DSN"])

	redis, ok := found["RedisSharedFixture"]
	gotest.True(t, ok, "expected RedisSharedFixture in discovered fixtures")
	gotest.Equal(t, "E2E_REDIS_ADDR", redis.EnvTags["Addr"])
}
