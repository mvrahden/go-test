package gotestgen

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/mvrahden/go-test/internal/gotestast"
	"github.com/mvrahden/go-test/pkg/gotest"
)

func TestGenerateSharedSetup_SingleFixture(t *testing.T) {
	fixtures := []SharedFixtureInfo{
		{
			Identifier:     "PostgresFixture",
			PkgPath:        "github.com/example/project/tests/fixtures",
			TransferFields: []string{"ConnStr"},
		},
	}

	src, err := GenerateSharedSetup(fixtures)
	gotest.NoError(t, err)
	gotest.NotEmpty(t, src)

	code := string(src)

	fset := token.NewFileSet()
	_, err = parser.ParseFile(fset, "setup.go", code, parser.AllErrors)
	gotest.NoError(t, err, "generated code should be valid Go: %s", code)

	gotest.Contains(t, code, "package main")
	gotest.Contains(t, code, `"context"`)
	gotest.Contains(t, code, `"encoding/json"`)
	gotest.Contains(t, code, `"os/signal"`)
	gotest.Contains(t, code, `"syscall"`)
	gotest.Contains(t, code, `sfpkg0 "github.com/example/project/tests/fixtures"`)
	gotest.Contains(t, code, "sfpkg0.PostgresFixture{}")
	gotest.Contains(t, code, "sf0.BeforeAll(ctx)")
	gotest.Contains(t, code, "sf0.AfterAll(ctx)")
	gotest.Contains(t, code, "json.NewEncoder(os.Stdout).Encode(state)")
	gotest.Contains(t, code, "signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)")
	gotest.Contains(t, code, "ConnStr: sf0.ConnStr")
}

func TestGenerateSharedSetup_MultipleFixtures(t *testing.T) {
	fixtures := []SharedFixtureInfo{
		{
			Identifier:     "PostgresFixture",
			PkgPath:        "github.com/example/project/tests/fixtures",
			TransferFields: []string{"ConnStr"},
		},
		{
			Identifier:     "RedisFixture",
			PkgPath:        "github.com/example/project/tests/redis",
			TransferFields: []string{"Addr"},
		},
	}

	src, err := GenerateSharedSetup(fixtures)
	gotest.NoError(t, err)
	gotest.NotEmpty(t, src)

	code := string(src)

	fset := token.NewFileSet()
	_, err = parser.ParseFile(fset, "setup.go", code, parser.AllErrors)
	gotest.NoError(t, err, "generated code should be valid Go: %s", code)

	gotest.Contains(t, code, `sfpkg0 "github.com/example/project/tests/fixtures"`)
	gotest.Contains(t, code, `sfpkg1 "github.com/example/project/tests/redis"`)

	gotest.Contains(t, code, "sfpkg0.PostgresFixture{}")
	gotest.Contains(t, code, "sfpkg1.RedisFixture{}")

	gotest.Contains(t, code, "sf0.BeforeAll(ctx)")
	gotest.Contains(t, code, "sf1.BeforeAll(ctx)")
	gotest.Contains(t, code, "sf0.AfterAll(ctx)")
	gotest.Contains(t, code, "sf1.AfterAll(ctx)")

	lastSf1 := strings.LastIndex(code, "sf1.AfterAll(ctx)")
	lastSf0 := strings.LastIndex(code, "sf0.AfterAll(ctx)")
	gotest.True(t, lastSf1 < lastSf0, "teardown should be in reverse order: sf1 before sf0")
}

func TestGenerateSharedSetup_NoFixtures(t *testing.T) {
	_, err := GenerateSharedSetup(nil)
	gotest.Error(t, err)
	gotest.Contains(t, err.Error(), "no shared fixtures")
}

func TestGenerateSharedSetup_NoTransferFields(t *testing.T) {
	fixtures := []SharedFixtureInfo{
		{
			Identifier:     "SetupFixture",
			PkgPath:        "github.com/example/fixtures",
			TransferFields: nil,
		},
	}

	src, err := GenerateSharedSetup(fixtures)
	gotest.NoError(t, err)

	code := string(src)
	fset := token.NewFileSet()
	_, err = parser.ParseFile(fset, "setup.go", code, parser.AllErrors)
	gotest.NoError(t, err, "generated code should be valid Go: %s", code)

	gotest.Contains(t, code, "sf0.BeforeAll(ctx)")
	gotest.Contains(t, code, "sf0.AfterAll(ctx)")
}

func TestDiscoverSharedFixtures_Basic(t *testing.T) {
	sf := gotestast.NewFixtureSpecForTestWithPkg("RedisFixture", gotestast.SharedFixture, "github.com/example/fixtures")
	sf.EnvTags = map[string]string{"Addr": "REDIS_ADDR"}

	results := []CollectorResult{
		{
			Fixtures: []*gotestast.FixtureSpec{sf},
		},
	}

	shared := DiscoverSharedFixtures(results)
	gotest.Equal(t, 1, len(shared))
	gotest.Equal(t, "RedisFixture", shared[0].Identifier)
	gotest.Equal(t, "github.com/example/fixtures", shared[0].PkgPath)
	gotest.Equal(t, "REDIS_ADDR", shared[0].EnvTags["Addr"])
	gotest.True(t, !shared[0].HasConfig)
	gotest.True(t, !shared[0].HasHydrate)
	gotest.True(t, !shared[0].HasDehydrate)
}

func TestDiscoverSharedFixtures_Deduplication(t *testing.T) {
	sf1 := gotestast.NewFixtureSpecForTestWithPkg("RedisFixture", gotestast.SharedFixture, "github.com/example/fixtures")
	sf2 := gotestast.NewFixtureSpecForTestWithPkg("RedisFixture", gotestast.SharedFixture, "github.com/example/fixtures")

	results := []CollectorResult{
		{Fixtures: []*gotestast.FixtureSpec{sf1}},
		{Fixtures: []*gotestast.FixtureSpec{sf2}},
	}

	shared := DiscoverSharedFixtures(results)
	gotest.Equal(t, 1, len(shared), "should deduplicate same shared fixture")
}

func TestDiscoverSharedFixtures_IgnoresPackageFixtures(t *testing.T) {
	pf := gotestast.NewFixtureSpecForTestWithPkg("DBFixture", gotestast.PackageFixture, "github.com/example/pkg")

	results := []CollectorResult{
		{Fixtures: []*gotestast.FixtureSpec{pf}},
	}

	shared := DiscoverSharedFixtures(results)
	gotest.Equal(t, 0, len(shared), "should not include package fixtures")
}

func TestDiscoverSharedFixtures_Empty(t *testing.T) {
	shared := DiscoverSharedFixtures(nil)
	gotest.Equal(t, 0, len(shared))
}

func TestGenerateSharedSetup_MultipleTransferFields(t *testing.T) {
	fixtures := []SharedFixtureInfo{
		{
			Identifier:     "PostgresFixture",
			PkgPath:        "github.com/example/fixtures",
			TransferFields: []string{"ConnStr", "Port"},
			LocalFields:    []string{"Pool"},
		},
	}

	src, err := GenerateSharedSetup(fixtures)
	gotest.NoError(t, err)

	code := string(src)
	fset := token.NewFileSet()
	_, err = parser.ParseFile(fset, "setup.go", code, parser.AllErrors)
	gotest.NoError(t, err, "generated code should be valid Go: %s", code)

	gotest.True(t, strings.Contains(code, "sf0.ConnStr"), "ConnStr should be serialized")
	gotest.True(t, strings.Contains(code, "sf0.Port"), "Port should be serialized")
	gotest.True(t, !strings.Contains(code, "sf0.Pool"), "Pool is local and should not be serialized")
}

func TestGenerateSharedSetup_ContextCalls(t *testing.T) {
	fixtures := []SharedFixtureInfo{
		{
			Identifier:     "PGFixture",
			PkgPath:        "github.com/example/fixtures",
			TransferFields: []string{"ConnStr"},
		},
	}

	src, err := GenerateSharedSetup(fixtures)
	gotest.NoError(t, err)

	code := string(src)
	gotest.Contains(t, code, "ctx := context.Background()")
	gotest.Contains(t, code, "sf0.BeforeAll(ctx)")
	gotest.Contains(t, code, "sf0.AfterAll(ctx)")
}

func TestGenerateSharedSetup_MarshalErrorHandling(t *testing.T) {
	fixtures := []SharedFixtureInfo{
		{
			Identifier:     "PGFixture",
			PkgPath:        "github.com/example/fixtures",
			TransferFields: []string{"ConnStr"},
		},
	}

	src, err := GenerateSharedSetup(fixtures)
	gotest.NoError(t, err)

	code := string(src)
	gotest.Contains(t, code, "json.Marshal(transfer{")
	gotest.Contains(t, code, "PGFixture: marshal:")
}

func TestGenerateSharedSetup_StateKeyUsesFullyQualifiedName(t *testing.T) {
	fixtures := []SharedFixtureInfo{
		{
			Identifier:     "PGFixture",
			PkgPath:        "github.com/example/fixtures",
			TransferFields: []string{"ConnStr"},
		},
	}

	src, err := GenerateSharedSetup(fixtures)
	gotest.NoError(t, err)

	code := string(src)
	gotest.Contains(t, code, `state["github.com/example/fixtures.PGFixture"]`)
}

func TestGenerateSharedSetup_ReverseOrderTeardown_OnError(t *testing.T) {
	fixtures := []SharedFixtureInfo{
		{
			Identifier:     "PGFixture",
			PkgPath:        "github.com/example/fixtures",
			TransferFields: []string{"ConnStr"},
		},
		{
			Identifier:     "RedisFixture",
			PkgPath:        "github.com/example/redis",
			TransferFields: []string{"Addr"},
		},
	}

	src, err := GenerateSharedSetup(fixtures)
	gotest.NoError(t, err)

	code := string(src)
	fset := token.NewFileSet()
	_, err = parser.ParseFile(fset, "setup.go", code, parser.AllErrors)
	gotest.NoError(t, err, "generated code should be valid Go: %s", code)

	// When sf1.BeforeAll fails, sf0 should be torn down
	idx1BeforeAll := strings.Index(code, "sf1.BeforeAll(ctx)")
	idxTeardownSf0 := strings.Index(code[idx1BeforeAll:], "sf0.AfterAll(ctx)")
	gotest.True(t, idxTeardownSf0 > 0, "sf0.AfterAll should appear after sf1.BeforeAll error block")
}
