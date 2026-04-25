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
			Identifier: "PostgresFixture",
			PkgPath:    "github.com/example/project/tests/fixtures",
			EnvTags: map[string]string{
				"ConnStr": "POSTGRES_URL",
			},
		},
	}

	src, err := GenerateSharedSetup(fixtures)
	gotest.NoError(t, err)
	gotest.NotEmpty(t, src)

	code := string(src)

	// Verify it's valid Go source
	fset := token.NewFileSet()
	_, err = parser.ParseFile(fset, "setup.go", code, parser.AllErrors)
	gotest.NoError(t, err, "generated code should be valid Go: %s", code)

	// Verify key elements are present
	gotest.Contains(t, code, "package main")
	gotest.Contains(t, code, `"encoding/json"`)
	gotest.Contains(t, code, `"os/signal"`)
	gotest.Contains(t, code, `"syscall"`)
	gotest.Contains(t, code, `sfpkg0 "github.com/example/project/tests/fixtures"`)
	gotest.Contains(t, code, "sfpkg0.PostgresFixture{}")
	gotest.Contains(t, code, "sf0.BeforeAll()")
	gotest.Contains(t, code, `env["POSTGRES_URL"]`)
	gotest.Contains(t, code, "json.NewEncoder(os.Stdout).Encode(env)")
	gotest.Contains(t, code, "signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)")
	gotest.Contains(t, code, "sf0.AfterAll()")
}

func TestGenerateSharedSetup_MultipleFixtures(t *testing.T) {
	fixtures := []SharedFixtureInfo{
		{
			Identifier: "PostgresFixture",
			PkgPath:    "github.com/example/project/tests/fixtures",
			EnvTags: map[string]string{
				"ConnStr": "POSTGRES_URL",
			},
		},
		{
			Identifier: "RedisFixture",
			PkgPath:    "github.com/example/project/tests/redis",
			EnvTags: map[string]string{
				"Addr": "REDIS_ADDR",
			},
		},
	}

	src, err := GenerateSharedSetup(fixtures)
	gotest.NoError(t, err)
	gotest.NotEmpty(t, src)

	code := string(src)

	// Verify it's valid Go source
	fset := token.NewFileSet()
	_, err = parser.ParseFile(fset, "setup.go", code, parser.AllErrors)
	gotest.NoError(t, err, "generated code should be valid Go: %s", code)

	// Both imports present
	gotest.Contains(t, code, `sfpkg0 "github.com/example/project/tests/fixtures"`)
	gotest.Contains(t, code, `sfpkg1 "github.com/example/project/tests/redis"`)

	// Both fixtures initialized
	gotest.Contains(t, code, "sfpkg0.PostgresFixture{}")
	gotest.Contains(t, code, "sfpkg1.RedisFixture{}")

	// Error handling in second fixture should teardown the first
	gotest.Contains(t, code, "sf0.AfterAll()")
	gotest.Contains(t, code, "sf1.AfterAll()")

	// Verify teardown in reverse order at the end
	// sf1.AfterAll() should appear before sf0.AfterAll() in the final teardown
	lastSf1 := strings.LastIndex(code, "sf1.AfterAll()")
	lastSf0 := strings.LastIndex(code, "sf0.AfterAll()")
	gotest.True(t, lastSf1 < lastSf0, "teardown should be in reverse order: sf1 before sf0")
}

func TestGenerateSharedSetup_NoFixtures(t *testing.T) {
	_, err := GenerateSharedSetup(nil)
	gotest.Error(t, err)
	gotest.Contains(t, err.Error(), "no shared fixtures")
}

func TestGenerateSharedSetup_NoEnvTags(t *testing.T) {
	fixtures := []SharedFixtureInfo{
		{
			Identifier: "SetupFixture",
			PkgPath:    "github.com/example/fixtures",
			EnvTags:    nil,
		},
	}

	src, err := GenerateSharedSetup(fixtures)
	gotest.NoError(t, err)

	// Verify it's valid Go source
	fset := token.NewFileSet()
	_, err = parser.ParseFile(fset, "setup.go", string(src), parser.AllErrors)
	gotest.NoError(t, err, "generated code should be valid Go: %s", string(src))
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
