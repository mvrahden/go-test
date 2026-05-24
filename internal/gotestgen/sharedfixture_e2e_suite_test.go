package gotestgen_test

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SharedFixtureE2ETestSuite tests shared-fixture code generation end-to-end
// across multi-package example directories.
type SharedFixtureE2ETestSuite struct{}

func (s *SharedFixtureE2ETestSuite) TestMultiPackage(t *gotest.T) {
	t.When("shared_fixture example directory exists", func(w *gotest.T) {
		w.It("generates correct per-package output and shared fixture setup", func(it *gotest.T) {
			goWorkFile := filepath.Join("..", "..", "go.work")
			if _, err := os.Stat(goWorkFile); os.IsNotExist(err) {
				it.T().Skip("go.work not found — run 'go work init . && go work use ./examples' at the project root to enable cross-module golden tests")
			}

			sharedFixtureDir := filepath.Join("..", "..", "examples", "shared_fixture")
			if _, err := os.Stat(sharedFixtureDir); os.IsNotExist(err) {
				it.T().Skip("examples/shared_fixture directory not found — shared fixture examples were removed")
			}

			pkg := "../../examples/shared_fixture/..."
			loaded, err := gotestgen.LoadPackages([]string{pkg}, nil)
			gotest.NoError(it, err)
			results, sharedFixtures, err := gotestgen.GenerateFromLoaded(loaded)
			gotest.NoError(it, err)

			sort.Slice(results, func(i, j int) bool {
				return results[i].PkgPath < results[j].PkgPath
			})

			var activeResults []*gotestgen.GenerateResult
			for _, r := range results {
				if len(r.PTest) > 0 || len(r.PXTest) > 0 {
					activeResults = append(activeResults, r)
				}
			}
			gotest.Equal(it, 2, len(activeResults), "expected 2 packages with generated code (api, web)")

			// PerPackageGoldenFiles
			for _, r := range activeResults {
				var subdir string
				switch {
				case strings.HasSuffix(r.PkgPath, "/api"):
					subdir = "api"
				case strings.HasSuffix(r.PkgPath, "/web"):
					subdir = "web"
				default:
					it.T().Fatalf("unexpected package: %s", r.PkgPath)
				}

				testdataDir := filepath.Join("..", "..", "examples", "shared_fixture", subdir, "testdata")
				ptestExpected := getExpectedOutputFile(it.T(), testdataDir, "gotestgen_ptest.golden")
				gotest.Equal(it, ptestExpected, string(r.PTest))
			}

			// APIPackage_MultiFixtureEmbedding
			var apiResult *gotestgen.GenerateResult
			for _, r := range activeResults {
				if strings.HasSuffix(r.PkgPath, "/api") {
					apiResult = r
					break
				}
			}
			gotest.True(it, apiResult != nil, "expected API package in results")
			code := string(apiResult.PTest)

			gotest.Contains(it, code, `"github.com/mvrahden/go-test/examples/shared_fixture"`)
			gotest.Contains(it, code, `"encoding/json"`)

			gotest.Contains(it, code, "sf0 := &sharedfixture.PostgresSharedFixture{}")
			gotest.Contains(it, code, `os.Getenv("GOTEST_SHARED_STATE_FILE")`)
			gotest.Contains(it, code, "ƒ_APIFixture.PostgresSharedFixture = sf0")

			gotest.Contains(it, code, "sf1 := &sharedfixture.RedisSharedFixture{}")
			gotest.Contains(it, code, "ƒ_APIFixture.RedisSharedFixture = sf1")

			// Hydrate should be called for Postgres (has Hydrate method)
			gotest.Contains(it, code, "sf0.Hydrate(context.Background())")
			// Dehydrate cleanup for Postgres
			gotest.Contains(it, code, "sf0.Dehydrate(context.Background())")
			// Redis has neither Hydrate nor Dehydrate
			gotest.True(it, !strings.Contains(code, "sf1.Hydrate"), "Redis should not have Hydrate call")
			gotest.True(it, !strings.Contains(code, "sf1.Dehydrate"), "Redis should not have Dehydrate call")

			sfResolvedBeforeFixture(it.T(), code, "json.Unmarshal(ƒb, sf0)", "ƒ_APIFixture.BeforeAll")

			// WebPackage_SingleFixtureEmbedding
			var webResult *gotestgen.GenerateResult
			for _, r := range activeResults {
				if strings.HasSuffix(r.PkgPath, "/web") {
					webResult = r
					break
				}
			}
			gotest.True(it, webResult != nil, "expected Web package in results")
			webCode := string(webResult.PTest)

			gotest.Contains(it, webCode, "sf0 := &sharedfixture.PostgresSharedFixture{}")
			gotest.Contains(it, webCode, "ƒ_WebFixture.PostgresSharedFixture = sf0")
			gotest.Contains(it, webCode, "sf0.Hydrate(context.Background())")

			gotest.True(it, !strings.Contains(webCode, "RedisSharedFixture"),
				"web package should NOT reference Redis")

			// SharedFixtureDiscovery
			shared := sharedFixtures
			gotest.Equal(it, 2, len(shared), "expected two shared fixtures discovered")

			found := map[string]gotestgen.SharedFixtureInfo{}
			for _, sf := range shared {
				found[sf.Identifier] = sf
			}

			pg, ok := found["PostgresSharedFixture"]
			gotest.True(it, ok, "expected PostgresSharedFixture")
			gotest.Equal(it, "github.com/mvrahden/go-test/examples/shared_fixture", pg.PkgPath)
			gotest.True(it, pg.HasHydrate, "Postgres should have Hydrate")
			gotest.True(it, pg.HasDehydrate, "Postgres should have Dehydrate")
			gotest.Equal(it, 4, len(pg.TransferFields), "DSN, Port, SSL, Tags should be transfer fields")
			gotest.Equal(it, "DSN", pg.TransferFields[0])
			gotest.Equal(it, "Port", pg.TransferFields[1])
			gotest.Equal(it, "SSL", pg.TransferFields[2])
			gotest.Equal(it, "Tags", pg.TransferFields[3])
			gotest.Equal(it, 2, len(pg.LocalFields), "Pool and Ready should be local")
			gotest.Equal(it, "Pool", pg.LocalFields[0])
			gotest.Equal(it, "Ready", pg.LocalFields[1])

			redis, ok := found["RedisSharedFixture"]
			gotest.True(it, ok, "expected RedisSharedFixture")
			gotest.Equal(it, "github.com/mvrahden/go-test/examples/shared_fixture", redis.PkgPath)
			gotest.True(it, !redis.HasHydrate, "Redis should NOT have Hydrate")
			gotest.True(it, !redis.HasDehydrate, "Redis should NOT have Dehydrate")
			gotest.Equal(it, 4, len(redis.TransferFields), "Addr, DB, Password, Replicas should be transfer fields")
			gotest.Equal(it, "Addr", redis.TransferFields[0])
			gotest.Equal(it, "DB", redis.TransferFields[1])
			gotest.Equal(it, "Password", redis.TransferFields[2])
			gotest.Equal(it, "Replicas", redis.TransferFields[3])
			gotest.Equal(it, 0, len(redis.LocalFields), "Redis has no local fields")

			// SetupBinaryGeneration
			setupSrc, err := gotestgen.GenerateSharedSetup(sharedFixtures)
			gotest.NoError(it, err)

			fset := token.NewFileSet()
			_, parseErr := parser.ParseFile(fset, "setup.go", string(setupSrc), parser.AllErrors)
			gotest.NoError(it, parseErr, "generated setup binary should be valid Go")

			setupCode := string(setupSrc)

			// Deduplicated import: same package, one alias
			gotest.Equal(it, 1, strings.Count(setupCode, `"github.com/mvrahden/go-test/examples/shared_fixture"`),
				"same-package fixtures should produce exactly one import")

			// Postgres transfer fields -- Pool and Ready excluded (local)
			gotest.True(it, strings.Contains(setupCode, "sf0.DSN"), "DSN should be serialized")
			gotest.True(it, strings.Contains(setupCode, "sf0.Port"), "Port should be serialized")
			gotest.True(it, strings.Contains(setupCode, "sf0.SSL"), "SSL (nested struct) should be serialized")
			gotest.True(it, strings.Contains(setupCode, "sf0.Tags"), "Tags (map) should be serialized")
			gotest.True(it, !strings.Contains(setupCode, "sf0.Pool"), "Pool should NOT be serialized (local field)")
			gotest.True(it, !strings.Contains(setupCode, "sf0.Ready"), "Ready should NOT be serialized (local field)")

			// Redis transfer fields -- all exported, including pointer and slice
			gotest.True(it, strings.Contains(setupCode, "sf1.Addr"), "Addr should be serialized")
			gotest.True(it, strings.Contains(setupCode, "sf1.DB"), "DB should be serialized")
			gotest.True(it, strings.Contains(setupCode, "sf1.Password"), "Password (pointer) should be serialized")
			gotest.True(it, strings.Contains(setupCode, "sf1.Replicas"), "Replicas (slice) should be serialized")

			// Reverse teardown order
			sf1TeardownIdx := strings.LastIndex(setupCode, "sf1.AfterAll(ctx)")
			sf0TeardownIdx := strings.LastIndex(setupCode, "sf0.AfterAll(ctx)")
			gotest.True(it, sf1TeardownIdx < sf0TeardownIdx,
				"reverse teardown: Redis (sf1) should tear down before Postgres (sf0)")

			// Parallel startup: on failure, succeeded fixtures get AfterAll
			gotest.Contains(it, setupCode, "if ƒerrs[0] == nil {")
			gotest.Contains(it, setupCode, "if ƒerrs[1] == nil {")

			// Golden file comparison
			setupGoldenDir := filepath.Join("..", "..", "examples", "shared_fixture", "testdata")
			setupExpected := getExpectedOutputFile(it.T(), setupGoldenDir, "gotestgen_setup.golden")
			gotest.Equal(it, setupExpected, setupCode)
		})
	})
}

func (s *SharedFixtureE2ETestSuite) TestDumpGolden(t *gotest.T) {
	t.When("DUMP_GOLDEN is set", func(w *gotest.T) {
		w.It("regenerates golden files", func(it *gotest.T) {
			if os.Getenv("DUMP_GOLDEN") != "1" {
				it.T().Skip("set DUMP_GOLDEN=1 to regenerate golden files")
			}

			goWorkFile := filepath.Join("..", "..", "go.work")
			if _, err := os.Stat(goWorkFile); os.IsNotExist(err) {
				it.T().Skip("go.work not found")
			}

			sharedFixtureDir := filepath.Join("..", "..", "examples", "shared_fixture")
			if _, err := os.Stat(sharedFixtureDir); os.IsNotExist(err) {
				it.T().Skip("examples/shared_fixture directory not found — shared fixture examples were removed")
			}

			pkg := "../../examples/shared_fixture/..."
			loaded, err := gotestgen.LoadPackages([]string{pkg}, nil)
			gotest.NoError(it, err)
			results, sharedFixtures, err := gotestgen.GenerateFromLoaded(loaded)
			gotest.NoError(it, err)

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
					writeGolden(it.T(), testdataDir, "gotestgen_ptest.golden", r.PTest)
				}
			}

			if len(sharedFixtures) > 0 {
				setupSrc, err := gotestgen.GenerateSharedSetup(sharedFixtures)
				gotest.NoError(it, err)
				setupDir := filepath.Join("..", "..", "examples", "shared_fixture", "testdata")
				os.MkdirAll(setupDir, 0755)
				writeGolden(it.T(), setupDir, "gotestgen_setup.golden", setupSrc)
			}

			it.T().Log("Golden files written. Re-run the main test to verify.")
		})
	})
}

// --- helpers ---

func sfResolvedBeforeFixture(t testing.TB, code, sfField, fixtureCall string) {
	t.Helper()
	sfIdx := strings.Index(code, sfField)
	fixtureIdx := strings.Index(code, fixtureCall)
	gotest.True(t, sfIdx > 0 && fixtureIdx > 0, "expected both %q and %q in code", sfField, fixtureCall)
	gotest.True(t, sfIdx < fixtureIdx,
		"shared fixture field %q should be resolved before %q", sfField, fixtureCall)
}

func writeGolden(t testing.TB, dir, name string, data []byte) {
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
