package gotestgen

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

func TestSharedFixture_E2E_MultiPackage(t *testing.T) {
	goWorkFile := filepath.Join("..", "..", "go.work")
	if _, err := os.Stat(goWorkFile); os.IsNotExist(err) {
		t.Skip("go.work not found — run 'go work init . && go work use ./examples' at the project root to enable cross-module golden tests")
	}

	pkg := "../../examples/shared_fixture/..."
	loaded, err := LoadPackages([]string{pkg}, nil)
	gotest.NoError(t, err)
	results, sharedFixtures, err := GenerateFromLoaded(loaded)
	gotest.NoError(t, err)

	sort.Slice(results, func(i, j int) bool {
		return results[i].PkgPath < results[j].PkgPath
	})

	var activeResults []*GenerateResult
	for _, r := range results {
		if len(r.PTest) > 0 || len(r.PXTest) > 0 {
			activeResults = append(activeResults, r)
		}
	}
	gotest.Equal(t, 2, len(activeResults), "expected 2 packages with generated code (api, web)")

	t.Run("PerPackageGoldenFiles", func(t *testing.T) {
		for _, r := range activeResults {
			var subdir string
			switch {
			case strings.HasSuffix(r.PkgPath, "/api"):
				subdir = "api"
			case strings.HasSuffix(r.PkgPath, "/web"):
				subdir = "web"
			default:
				t.Fatalf("unexpected package: %s", r.PkgPath)
			}

			t.Run(subdir, func(t *testing.T) {
				testdataDir := filepath.Join("..", "..", "examples", "shared_fixture", subdir, "testdata")
				ptestExpected := getExpectedOutputFile(t, testdataDir, "gotestgen_ptest.golden")
				gotest.Equal(t, ptestExpected, string(r.PTest))
			})
		}
	})

	t.Run("APIPackage_MultiFixtureEmbedding", func(t *testing.T) {
		var apiResult *GenerateResult
		for _, r := range activeResults {
			if strings.HasSuffix(r.PkgPath, "/api") {
				apiResult = r
				break
			}
		}
		gotest.True(t, apiResult != nil, "expected API package in results")
		code := string(apiResult.PTest)

		gotest.Contains(t, code, `"github.com/mvrahden/go-test/examples/shared_fixture"`)
		gotest.Contains(t, code, `"encoding/json"`)

		gotest.Contains(t, code, "sf0 := &sharedfixture.PostgresSharedFixture{}")
		gotest.Contains(t, code, `os.Getenv("GOTEST_SHARED_STATE_FILE")`)
		gotest.Contains(t, code, "ƒ_APIFixture.PostgresSharedFixture = sf0")

		gotest.Contains(t, code, "sf1 := &sharedfixture.RedisSharedFixture{}")
		gotest.Contains(t, code, "ƒ_APIFixture.RedisSharedFixture = sf1")

		// Hydrate should be called for Postgres (has Hydrate method)
		gotest.Contains(t, code, "sf0.Hydrate(context.Background())")
		// Dehydrate cleanup for Postgres
		gotest.Contains(t, code, "sf0.Dehydrate(context.Background())")
		// Redis has neither Hydrate nor Dehydrate
		gotest.True(t, !strings.Contains(code, "sf1.Hydrate"), "Redis should not have Hydrate call")
		gotest.True(t, !strings.Contains(code, "sf1.Dehydrate"), "Redis should not have Dehydrate call")

		sfResolvedBeforeFixture(t, code, "json.Unmarshal(ƒb, sf0)", "ƒ_APIFixture.BeforeAll")
	})

	t.Run("WebPackage_SingleFixtureEmbedding", func(t *testing.T) {
		var webResult *GenerateResult
		for _, r := range activeResults {
			if strings.HasSuffix(r.PkgPath, "/web") {
				webResult = r
				break
			}
		}
		gotest.True(t, webResult != nil, "expected Web package in results")
		code := string(webResult.PTest)

		gotest.Contains(t, code, "sf0 := &sharedfixture.PostgresSharedFixture{}")
		gotest.Contains(t, code, "ƒ_WebFixture.PostgresSharedFixture = sf0")
		gotest.Contains(t, code, "sf0.Hydrate(context.Background())")

		gotest.True(t, !strings.Contains(code, "RedisSharedFixture"),
			"web package should NOT reference Redis")
	})

	t.Run("SharedFixtureDiscovery", func(t *testing.T) {
		shared := sharedFixtures
		gotest.Equal(t, 2, len(shared), "expected two shared fixtures discovered")

		found := map[string]SharedFixtureInfo{}
		for _, sf := range shared {
			found[sf.Identifier] = sf
		}

		pg, ok := found["PostgresSharedFixture"]
		gotest.True(t, ok, "expected PostgresSharedFixture")
		gotest.Equal(t, "github.com/mvrahden/go-test/examples/shared_fixture", pg.PkgPath)
		gotest.True(t, pg.HasHydrate, "Postgres should have Hydrate")
		gotest.True(t, pg.HasDehydrate, "Postgres should have Dehydrate")
		gotest.Equal(t, 4, len(pg.TransferFields), "DSN, Port, SSL, Tags should be transfer fields")
		gotest.Equal(t, "DSN", pg.TransferFields[0])
		gotest.Equal(t, "Port", pg.TransferFields[1])
		gotest.Equal(t, "SSL", pg.TransferFields[2])
		gotest.Equal(t, "Tags", pg.TransferFields[3])
		gotest.Equal(t, 2, len(pg.LocalFields), "Pool and Ready should be local")
		gotest.Equal(t, "Pool", pg.LocalFields[0])
		gotest.Equal(t, "Ready", pg.LocalFields[1])

		redis, ok := found["RedisSharedFixture"]
		gotest.True(t, ok, "expected RedisSharedFixture")
		gotest.Equal(t, "github.com/mvrahden/go-test/examples/shared_fixture", redis.PkgPath)
		gotest.True(t, !redis.HasHydrate, "Redis should NOT have Hydrate")
		gotest.True(t, !redis.HasDehydrate, "Redis should NOT have Dehydrate")
		gotest.Equal(t, 4, len(redis.TransferFields), "Addr, DB, Password, Replicas should be transfer fields")
		gotest.Equal(t, "Addr", redis.TransferFields[0])
		gotest.Equal(t, "DB", redis.TransferFields[1])
		gotest.Equal(t, "Password", redis.TransferFields[2])
		gotest.Equal(t, "Replicas", redis.TransferFields[3])
		gotest.Equal(t, 0, len(redis.LocalFields), "Redis has no local fields")
	})

	t.Run("SetupBinaryGeneration", func(t *testing.T) {
		setupSrc, err := GenerateSharedSetup(sharedFixtures)
		gotest.NoError(t, err)

		fset := token.NewFileSet()
		_, parseErr := parser.ParseFile(fset, "setup.go", string(setupSrc), parser.AllErrors)
		gotest.NoError(t, parseErr, "generated setup binary should be valid Go")

		code := string(setupSrc)

		// Deduplicated import: same package, one alias
		gotest.Equal(t, 1, strings.Count(code, `"github.com/mvrahden/go-test/examples/shared_fixture"`),
			"same-package fixtures should produce exactly one import")

		// Postgres transfer fields — Pool and Ready excluded (local)
		gotest.True(t, strings.Contains(code, "sf0.DSN"), "DSN should be serialized")
		gotest.True(t, strings.Contains(code, "sf0.Port"), "Port should be serialized")
		gotest.True(t, strings.Contains(code, "sf0.SSL"), "SSL (nested struct) should be serialized")
		gotest.True(t, strings.Contains(code, "sf0.Tags"), "Tags (map) should be serialized")
		gotest.True(t, !strings.Contains(code, "sf0.Pool"), "Pool should NOT be serialized (local field)")
		gotest.True(t, !strings.Contains(code, "sf0.Ready"), "Ready should NOT be serialized (local field)")

		// Redis transfer fields — all exported, including pointer and slice
		gotest.True(t, strings.Contains(code, "sf1.Addr"), "Addr should be serialized")
		gotest.True(t, strings.Contains(code, "sf1.DB"), "DB should be serialized")
		gotest.True(t, strings.Contains(code, "sf1.Password"), "Password (pointer) should be serialized")
		gotest.True(t, strings.Contains(code, "sf1.Replicas"), "Replicas (slice) should be serialized")

		// Reverse teardown order
		sf1TeardownIdx := strings.LastIndex(code, "sf1.AfterAll(ctx)")
		sf0TeardownIdx := strings.LastIndex(code, "sf0.AfterAll(ctx)")
		gotest.True(t, sf1TeardownIdx < sf0TeardownIdx,
			"reverse teardown: Redis (sf1) should tear down before Postgres (sf0)")

		// Parallel startup: on failure, succeeded fixtures get AfterAll
		gotest.Contains(t, code, "if ƒerrs[0] == nil {")
		gotest.Contains(t, code, "if ƒerrs[1] == nil {")

		// Golden file comparison
		setupGoldenDir := filepath.Join("..", "..", "examples", "shared_fixture", "testdata")
		setupExpected := getExpectedOutputFile(t, setupGoldenDir, "gotestgen_setup.golden")
		gotest.Equal(t, setupExpected, code)
	})
}

func sfResolvedBeforeFixture(t *testing.T, code, sfField, fixtureCall string) {
	t.Helper()
	sfIdx := strings.Index(code, sfField)
	fixtureIdx := strings.Index(code, fixtureCall)
	gotest.True(t, sfIdx > 0 && fixtureIdx > 0, "expected both %q and %q in code", sfField, fixtureCall)
	gotest.True(t, sfIdx < fixtureIdx,
		"shared fixture field %q should be resolved before %q", sfField, fixtureCall)
}

func TestSharedFixture_E2E_DumpGolden(t *testing.T) {
	if os.Getenv("DUMP_GOLDEN") != "1" {
		t.Skip("set DUMP_GOLDEN=1 to regenerate golden files")
	}

	goWorkFile := filepath.Join("..", "..", "go.work")
	if _, err := os.Stat(goWorkFile); os.IsNotExist(err) {
		t.Skip("go.work not found")
	}

	pkg := "../../examples/shared_fixture/..."
	loaded, err := LoadPackages([]string{pkg}, nil)
	gotest.NoError(t, err)
	results, sharedFixtures, err := GenerateFromLoaded(loaded)
	gotest.NoError(t, err)

	for _, r := range results {
		var subdir string
		switch {
		case strings.HasSuffix(r.PkgPath, "/api"):
			subdir = "api"
		case strings.HasSuffix(r.PkgPath, "/web"):
			subdir = "web"
		default:
			continue
		}

		testdataDir := filepath.Join("..", "..", "examples", "shared_fixture", subdir, "testdata")
		os.MkdirAll(testdataDir, 0755)

		if len(r.PTest) > 0 {
			writeGolden(t, testdataDir, "gotestgen_ptest.golden", r.PTest)
		}
	}

	if len(sharedFixtures) > 0 {
		setupSrc, err := GenerateSharedSetup(sharedFixtures)
		gotest.NoError(t, err)
		setupDir := filepath.Join("..", "..", "examples", "shared_fixture", "testdata")
		os.MkdirAll(setupDir, 0755)
		writeGolden(t, setupDir, "gotestgen_setup.golden", setupSrc)
	}

	t.Log("Golden files written. Re-run the main test to verify.")
}

func writeGolden(t *testing.T, dir, name string, data []byte) {
	t.Helper()
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	fmt.Fprintf(os.Stderr, "wrote %s (%d bytes)\n", path, len(data))
}
