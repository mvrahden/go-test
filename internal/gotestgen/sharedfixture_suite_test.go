package gotestgen_test

import (
	"go/parser"
	"go/token"
	"slices"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SharedFixtureTestSuite tests shared-fixture setup binary generation
// from SharedFixtureInfo inputs.
type SharedFixtureTestSuite struct{}

func (s *SharedFixtureTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *SharedFixtureTestSuite) TestGenerateSharedSetup(t *gotest.T) {
	t.When("single fixture with one transfer field", func(w *gotest.T) {
		w.It("generates valid Go with expected structure", func(it *gotest.T) {
			fixtures := []gotestgen.SharedFixtureInfo{
				{
					Identifier:     "PostgresFixture",
					HasAfterAll:    true,
					PkgPath:        "github.com/example/project/tests/fixtures",
					TransferFields: []string{"ConnStr"},
				},
			}

			src, err := gotestgen.GenerateSharedSetup(fixtures)
			gotest.NoError(it, err)
			gotest.NotEmpty(it, src)

			code := string(src)

			fset := token.NewFileSet()
			_, err = parser.ParseFile(fset, "setup.go", code, parser.AllErrors)
			gotest.NoError(it, err, "generated code should be valid Go: %s", code)

			gotest.MatchSnapshot(it, code)
		})
	})

	t.When("multiple fixtures from different packages", func(w *gotest.T) {
		w.It("generates imports and lifecycle for both", func(it *gotest.T) {
			fixtures := []gotestgen.SharedFixtureInfo{
				{
					Identifier:     "PostgresFixture",
					HasAfterAll:    true,
					PkgPath:        "github.com/example/project/tests/fixtures",
					TransferFields: []string{"ConnStr"},
				},
				{
					Identifier:     "RedisFixture",
					HasAfterAll:    true,
					PkgPath:        "github.com/example/project/tests/redis",
					TransferFields: []string{"Addr"},
				},
			}

			src, err := gotestgen.GenerateSharedSetup(fixtures)
			gotest.NoError(it, err)
			gotest.NotEmpty(it, src)

			code := string(src)

			fset := token.NewFileSet()
			_, err = parser.ParseFile(fset, "setup.go", code, parser.AllErrors)
			gotest.NoError(it, err, "generated code should be valid Go: %s", code)

			gotest.MatchSnapshot(it, code)
		})
	})

	t.When("no fixtures", func(w *gotest.T) {
		w.It("returns an error", func(it *gotest.T) {
			_, err := gotestgen.GenerateSharedSetup(nil)
			gotest.ErrorContains(it, err, "no shared fixtures")
		})
	})

	t.When("no transfer fields", func(w *gotest.T) {
		w.It("generates valid Go with lifecycle calls", func(it *gotest.T) {
			fixtures := []gotestgen.SharedFixtureInfo{
				{
					Identifier:     "SetupFixture",
					HasAfterAll:    true,
					PkgPath:        "github.com/example/fixtures",
					TransferFields: nil,
				},
			}

			src, err := gotestgen.GenerateSharedSetup(fixtures)
			gotest.NoError(it, err)

			code := string(src)
			fset := token.NewFileSet()
			_, err = parser.ParseFile(fset, "setup.go", code, parser.AllErrors)
			gotest.NoError(it, err, "generated code should be valid Go: %s", code)

			gotest.MatchSnapshot(it, code)
		})
	})

	t.When("multiple transfer and local fields", func(w *gotest.T) {
		w.It("serializes only transfer fields", func(it *gotest.T) {
			fixtures := []gotestgen.SharedFixtureInfo{
				{
					Identifier:     "PostgresFixture",
					HasAfterAll:    true,
					PkgPath:        "github.com/example/fixtures",
					TransferFields: []string{"ConnStr", "Port"},
					LocalFields:    []string{"Pool"},
				},
			}

			src, err := gotestgen.GenerateSharedSetup(fixtures)
			gotest.NoError(it, err)

			code := string(src)
			fset := token.NewFileSet()
			_, err = parser.ParseFile(fset, "setup.go", code, parser.AllErrors)
			gotest.NoError(it, err, "generated code should be valid Go: %s", code)

			gotest.MatchSnapshot(it, code)

			gotest.NotContains(it, code, "sf0.Pool", "Pool is local and should not be serialized")
		})
	})

	t.When("two fixtures from same package", func(w *gotest.T) {
		w.It("deduplicates the import", func(it *gotest.T) {
			fixtures := []gotestgen.SharedFixtureInfo{
				{
					Identifier:     "PostgresFixture",
					HasAfterAll:    true,
					PkgPath:        "github.com/example/shared",
					TransferFields: []string{"DSN"},
				},
				{
					Identifier:     "RedisFixture",
					HasAfterAll:    true,
					PkgPath:        "github.com/example/shared",
					TransferFields: []string{"Addr"},
				},
			}

			src, err := gotestgen.GenerateSharedSetup(fixtures)
			gotest.NoError(it, err)

			code := string(src)
			fset := token.NewFileSet()
			_, err = parser.ParseFile(fset, "setup.go", code, parser.AllErrors)
			gotest.NoError(it, err, "generated code should be valid Go: %s", code)

			gotest.MatchSnapshot(it, code)

			gotest.Equal(it, 1, strings.Count(code, `"github.com/example/shared"`),
				"same-package fixtures should produce exactly one import")
			gotest.NotContains(it, code, "sfpkg1",
				"should not have sfpkg1 alias when both fixtures share the same package")
		})
	})
}

func (s *SharedFixtureTestSuite) TestGeneratedCodeStructure(t *gotest.T) {
	t.When("context lifecycle", func(w *gotest.T) {
		w.It("generates context-aware lifecycle calls", func(it *gotest.T) {
			fixtures := []gotestgen.SharedFixtureInfo{
				{
					Identifier:     "PGFixture",
					HasAfterAll:    true,
					PkgPath:        "github.com/example/fixtures",
					TransferFields: []string{"ConnStr"},
				},
			}

			src, err := gotestgen.GenerateSharedSetup(fixtures)
			gotest.NoError(it, err)

			gotest.MatchSnapshot(it, string(src))
		})
	})

	t.When("fixture declares no AfterAll", func(w *gotest.T) {
		w.It("tears down without calling one", func(it *gotest.T) {
			fixtures := []gotestgen.SharedFixtureInfo{
				{
					Identifier:     "PGFixture",
					PkgPath:        "github.com/example/fixtures",
					TransferFields: []string{"ConnStr"},
				},
			}

			src, err := gotestgen.GenerateSharedSetup(fixtures)
			gotest.NoError(it, err)

			// AfterAll is optional; referencing it fails the setup build.
			gotest.NotContains(it, string(src), "sf0.AfterAll")
		})
	})

	t.When("default timeout", func(w *gotest.T) {
		w.It("uses default fixture config", func(it *gotest.T) {
			fixtures := []gotestgen.SharedFixtureInfo{
				{
					Identifier:     "PGFixture",
					HasAfterAll:    true,
					PkgPath:        "github.com/example/fixtures",
					TransferFields: []string{"ConnStr"},
				},
			}

			src, err := gotestgen.GenerateSharedSetup(fixtures)
			gotest.NoError(it, err)

			code := string(src)
			fset := token.NewFileSet()
			_, err = parser.ParseFile(fset, "setup.go", code, parser.AllErrors)
			gotest.NoError(it, err, "generated code should be valid Go: %s", code)

			gotest.MatchSnapshot(it, code)

			gotest.NotContains(it, code, "SharedFixtureConfig",
				"should not name SharedFixtureConfig when HasConfig is false")
		})
	})

	t.When("with config marker", func(w *gotest.T) {
		w.It("uses the marker config for the timeout", func(it *gotest.T) {
			fixtures := []gotestgen.SharedFixtureInfo{
				{
					Identifier:     "PGFixture",
					HasAfterAll:    true,
					PkgPath:        "github.com/example/fixtures",
					HasConfig:      true,
					TransferFields: []string{"ConnStr"},
				},
			}

			src, err := gotestgen.GenerateSharedSetup(fixtures)
			gotest.NoError(it, err)

			code := string(src)
			fset := token.NewFileSet()
			_, err = parser.ParseFile(fset, "setup.go", code, parser.AllErrors)
			gotest.NoError(it, err, "generated code should be valid Go: %s", code)

			// A bare marker call in main would panic before the handshake,
			// killing the process attributed to nothing: the runtime calls it.
			gotest.Contains(it, code, "Config:    sf0.SharedFixtureConfig,",
				"the config marker is handed over as a method value")
			gotest.NotContains(it, code, "SharedFixtureConfig()",
				"the marker must never be called by the generated program")

			gotest.MatchSnapshot(it, code)
		})
	})

	t.When("single fixture with one transfer field", func(w *gotest.T) {
		w.It("hands the runtime the fixture's name, key, hooks and state", func(it *gotest.T) {
			src, err := gotestgen.GenerateSharedSetup([]gotestgen.SharedFixtureInfo{{
				Identifier:     "PostgresFixture",
				HasAfterAll:    true,
				PkgPath:        "github.com/example/project/tests/fixtures",
				TransferFields: []string{"ConnStr"},
			}})
			gotest.NoError(it, err)
			code := string(src)
			gotest.Contains(it, code, "gotestruntime.RunSharedFixtures(gotestruntime.SharedProgram{Fixtures: []gotestruntime.SharedFixture{")
			gotest.Contains(it, code, `Name:      "PostgresFixture",`)
			gotest.Contains(it, code, `Key:       "github.com/example/project/tests/fixtures.PostgresFixture",`)
			gotest.Contains(it, code, "BeforeAll: sf0.BeforeAll,")
			gotest.Contains(it, code, "AfterAll:  sf0.AfterAll,")
			gotest.Regexp(it, "State: func\\(\\) any \\{\\s+return struct \\{\\s+ConnStr any `json:\"ConnStr\"`\\s+\\}\\{sf0\\.ConnStr\\}", code)
		})
	})

	t.When("no transfer fields", func(w *gotest.T) {
		w.It("hands over no state", func(it *gotest.T) {
			src, err := gotestgen.GenerateSharedSetup([]gotestgen.SharedFixtureInfo{{
				Identifier: "SetupFixture", HasAfterAll: true, PkgPath: "github.com/example/fixtures",
			}})
			gotest.NoError(it, err)
			gotest.NotContains(it, string(src), "State:")
		})
	})

	t.When("a fixture binds another", func(w *gotest.T) {
		w.It("names its dependency's key and wires the field", func(it *gotest.T) {
			src, err := gotestgen.GenerateSharedSetup([]gotestgen.SharedFixtureInfo{
				{Identifier: "DagASharedFixture", HasAfterAll: true, PkgPath: "example.com/fx"},
				{
					Identifier:       "DagChildSharedFixture",
					HasAfterAll:      true,
					PkgPath:          "example.com/fx",
					Dependencies:     []string{"example.com/fx.DagASharedFixture", "example.com/other.Unknown"},
					DependencyFields: map[string]string{"example.com/fx.DagASharedFixture": "Parent"},
				},
			})
			gotest.NoError(it, err)
			code := string(src)
			gotest.Contains(it, code, `DependsOn: []string{"example.com/fx.DagASharedFixture"},`)
			gotest.Regexp(it, `Wire: func\(\) \{\s+sf1\.Parent = sf0\s+\},`, code)
		})
	})

	t.When("a fixture is deferred", func(w *gotest.T) {
		w.It("hands it over as deferred", func(it *gotest.T) {
			src, err := gotestgen.GenerateSharedSetup([]gotestgen.SharedFixtureInfo{{
				Identifier: "PGFixture", HasAfterAll: true, PkgPath: "github.com/example/fixtures", Deferred: true,
			}})
			gotest.NoError(it, err)
			gotest.Contains(it, string(src), "Deferred:  true,")
		})
	})

	t.When("the program holds no logic of its own", func(w *gotest.T) {
		w.It("declares no function and starts no goroutine, and calls the runtime once", func(it *gotest.T) {
			src, err := gotestgen.GenerateSharedSetup([]gotestgen.SharedFixtureInfo{
				{Identifier: "PGFixture", HasAfterAll: true, HasConfig: true, PkgPath: "github.com/example/fixtures", TransferFields: []string{"ConnStr"}},
				{Identifier: "RedisFixture", PkgPath: "github.com/example/redis", Deferred: true},
			})
			gotest.NoError(it, err)
			code := string(src)
			gotest.NotContains(it, code, "func ƒ")
			gotest.NotContains(it, code, "go func")
			gotest.Equal(it, 1, strings.Count(code, "gotestruntime."+"RunSharedFixtures"))
		})
	})

	t.When("imports", func(w *gotest.T) {
		w.It("declares exactly what the generated program still uses", func(it *gotest.T) {
			fixtures := []gotestgen.SharedFixtureInfo{
				{
					Identifier:     "PGFixture",
					HasAfterAll:    true,
					PkgPath:        "github.com/example/fixtures",
					HasConfig:      true,
					TransferFields: []string{"ConnStr"},
				},
			}

			src, err := gotestgen.GenerateSharedSetup(fixtures)
			gotest.NoError(it, err)

			code := string(src)

			// format.Source does not type-check, so a stale import renders
			// cleanly here and only fails when the program is built in a
			// throwaway directory during a real run. Assert on the import block
			// itself rather than on "rendering returned no error".
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "setup.go", code, parser.ImportsOnly)
			gotest.NoError(it, err)

			var paths, locals []string
			for _, imp := range file.Imports {
				path := strings.Trim(imp.Path.Value, `"`)
				paths = append(paths, path)
				if imp.Name != nil {
					locals = append(locals, imp.Name.Name)
				} else {
					locals = append(locals, path[strings.LastIndex(path, "/")+1:])
				}
			}
			// The runtime and the fixtures' packages, nothing else.
			gotest.Equal(it, []string{
				"github.com/example/fixtures",
				"github.com/mvrahden/go-test/pkg/gotestruntime",
			}, slices.Sorted(slices.Values(paths)))

			funcIdx := strings.Index(code, "\nfunc ")
			gotest.NotEqual(it, -1, funcIdx, "generated program declares no functions")
			body := code[funcIdx:]
			for i, local := range locals {
				gotest.Contains(it, body, local+".",
					"import %q is declared but never used; the generated program will not build", paths[i])
			}
		})
	})

	t.When("state key format", func(w *gotest.T) {
		w.It("uses fully qualified package path and identifier", func(it *gotest.T) {
			fixtures := []gotestgen.SharedFixtureInfo{
				{
					Identifier:     "PGFixture",
					HasAfterAll:    true,
					PkgPath:        "github.com/example/fixtures",
					TransferFields: []string{"ConnStr"},
				},
			}

			src, err := gotestgen.GenerateSharedSetup(fixtures)
			gotest.NoError(it, err)

			gotest.MatchSnapshot(it, string(src))
		})
	})

	t.When("marshal error handling", func(w *gotest.T) {
		w.It("generates error handling for marshal", func(it *gotest.T) {
			fixtures := []gotestgen.SharedFixtureInfo{
				{
					Identifier:     "PGFixture",
					HasAfterAll:    true,
					PkgPath:        "github.com/example/fixtures",
					TransferFields: []string{"ConnStr"},
				},
			}

			src, err := gotestgen.GenerateSharedSetup(fixtures)
			gotest.NoError(it, err)

			gotest.MatchSnapshot(it, string(src))
		})
	})

	t.When("determinism", func(w *gotest.T) {
		w.It("generates byte-identical output on repeated runs", func(it *gotest.T) {
			// Two parent assignments is the smallest input where Go map
			// iteration order could shuffle the emitted statements.
			fixtures := []gotestgen.SharedFixtureInfo{
				{
					Identifier:     "AlphaSharedFixture",
					HasAfterAll:    true,
					PkgPath:        "github.com/example/fixtures",
					TransferFields: []string{"A"},
				},
				{
					Identifier:     "BetaSharedFixture",
					HasAfterAll:    true,
					PkgPath:        "github.com/example/fixtures",
					TransferFields: []string{"B"},
				},
				{
					Identifier:  "GammaSharedFixture",
					HasAfterAll: true,
					PkgPath:     "github.com/example/fixtures",
					Dependencies: []string{
						"github.com/example/fixtures.AlphaSharedFixture",
						"github.com/example/fixtures.BetaSharedFixture",
					},
					DependencyFields: map[string]string{
						"github.com/example/fixtures.AlphaSharedFixture": "Alpha",
						"github.com/example/fixtures.BetaSharedFixture":  "Beta",
					},
					TransferFields: []string{"C"},
				},
			}

			first, err := gotestgen.GenerateSharedSetup(fixtures)
			gotest.NoError(it, err)
			for range 25 {
				again, err := gotestgen.GenerateSharedSetup(fixtures)
				gotest.NoError(it, err)
				gotest.Equal(it, string(first), string(again),
					"generated code must be a pure function of its input; map order shuffled it run to run")
			}
		})
	})

	t.When("reverse teardown on error", func(w *gotest.T) {
		w.It("tears down sf0 in reverse order", func(it *gotest.T) {
			fixtures := []gotestgen.SharedFixtureInfo{
				{
					Identifier:     "PGFixture",
					HasAfterAll:    true,
					PkgPath:        "github.com/example/fixtures",
					TransferFields: []string{"ConnStr"},
				},
				{
					Identifier:     "RedisFixture",
					HasAfterAll:    true,
					PkgPath:        "github.com/example/redis",
					TransferFields: []string{"Addr"},
				},
			}

			src, err := gotestgen.GenerateSharedSetup(fixtures)
			gotest.NoError(it, err)

			code := string(src)
			fset := token.NewFileSet()
			_, err = parser.ParseFile(fset, "setup.go", code, parser.AllErrors)
			gotest.NoError(it, err, "generated code should be valid Go: %s", code)

			gotest.MatchSnapshot(it, code)
		})
	})
}

func (s *SharedFixtureTestSuite) TestIntegrationGeneratedSetupBinary(t *gotest.T) {
	t.When("multi-fixture setup binary", func(w *gotest.T) {
		w.It("generates valid Go with correct structure", func(it *gotest.T) {
			fixtures := []gotestgen.SharedFixtureInfo{
				{
					Identifier:     "PostgresFixture",
					HasAfterAll:    true,
					PkgPath:        "github.com/example/project/tests/fixtures",
					TransferFields: []string{"DSN"},
				},
				{
					Identifier:     "GCSFixture",
					HasAfterAll:    true,
					PkgPath:        "github.com/example/project/tests/gcs",
					TransferFields: []string{"Endpoint", "Bucket"},
				},
			}

			src, err := gotestgen.GenerateSharedSetup(fixtures)
			gotest.NoError(it, err)

			code := string(src)

			fset := token.NewFileSet()
			_, parseErr := parser.ParseFile(fset, "setup.go", code, parser.AllErrors)
			gotest.NoError(it, parseErr, "generated code should be valid Go: %s", code)

			gotest.MatchSnapshot(it, code)
		})
	})
}
