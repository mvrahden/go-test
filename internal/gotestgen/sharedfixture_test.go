package gotestgen //nolint:stdlib-test

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"

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

func TestGenerateSharedSetup_DefaultTimeout(t *testing.T) {
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
	fset := token.NewFileSet()
	_, err = parser.ParseFile(fset, "setup.go", code, parser.AllErrors)
	gotest.NoError(t, err, "generated code should be valid Go: %s", code)

	gotest.Contains(t, code, `gotest "github.com/mvrahden/go-test/pkg/gotest"`)
	gotest.Contains(t, code, "gotest.DefaultFixtureConfig()")
	gotest.Contains(t, code, "context.WithTimeout(ctx,")
	gotest.True(t, !strings.Contains(code, "SharedFixtureConfig()"),
		"should not call SharedFixtureConfig when HasConfig is false")
}

func TestGenerateSharedSetup_WithConfig(t *testing.T) {
	fixtures := []SharedFixtureInfo{
		{
			Identifier:     "PGFixture",
			PkgPath:        "github.com/example/fixtures",
			HasConfig:      true,
			TransferFields: []string{"ConnStr"},
		},
	}

	src, err := GenerateSharedSetup(fixtures)
	gotest.NoError(t, err)

	code := string(src)
	fset := token.NewFileSet()
	_, err = parser.ParseFile(fset, "setup.go", code, parser.AllErrors)
	gotest.NoError(t, err, "generated code should be valid Go: %s", code)

	gotest.Contains(t, code, "gotest.DefaultFixtureConfig()")
	gotest.Contains(t, code, "sf0.SharedFixtureConfig()")
	gotest.Contains(t, code, "gotest.OverlayFixtureConfig(")
	gotest.Contains(t, code, "context.WithTimeout(ctx,")
}

func TestGenerateSharedSetup_RetryLogic(t *testing.T) {
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
	gotest.Contains(t, code, "1 + ƒcfg_sf0.Retries")
	gotest.Contains(t, code, "time.Sleep(ƒcfg_sf0.RetryDelay)")
	gotest.Contains(t, code, "BeforeAll failed after")
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

func TestGenerateSharedSetup_SamePackageDeduplicatesImport(t *testing.T) {
	fixtures := []SharedFixtureInfo{
		{
			Identifier:     "PostgresFixture",
			PkgPath:        "github.com/example/shared",
			TransferFields: []string{"DSN"},
		},
		{
			Identifier:     "RedisFixture",
			PkgPath:        "github.com/example/shared",
			TransferFields: []string{"Addr"},
		},
	}

	src, err := GenerateSharedSetup(fixtures)
	gotest.NoError(t, err)

	code := string(src)
	fset := token.NewFileSet()
	_, err = parser.ParseFile(fset, "setup.go", code, parser.AllErrors)
	gotest.NoError(t, err, "generated code should be valid Go: %s", code)

	gotest.Equal(t, 1, strings.Count(code, `"github.com/example/shared"`),
		"same-package fixtures should produce exactly one import")

	gotest.Contains(t, code, "sfpkg0.PostgresFixture{}")
	gotest.Contains(t, code, "sfpkg0.RedisFixture{}")
	gotest.True(t, !strings.Contains(code, "sfpkg1"),
		"should not have sfpkg1 alias when both fixtures share the same package")
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
