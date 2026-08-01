package gotestgen_test

import (
	"go/parser"
	"go/token"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SharedFixtureTestSuite tests shared-fixture setup binary generation
// from SharedFixtureInfo inputs.
type SharedFixtureTestSuite struct{}

func (s *SharedFixtureTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *SharedFixtureTestSuite) TestGenerateSharedSetup(t *gotest.T) {
	t.When("single fixture with one transfer field", func(w *gotest.T) {
		w.It("generates valid Go with expected structure", func(it *gotest.T) {
			fixtures := []gotestgen.SharedFixtureInfo{
				{
					Identifier:     "PostgresFixture",
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
					PkgPath:        "github.com/example/project/tests/fixtures",
					TransferFields: []string{"ConnStr"},
				},
				{
					Identifier:     "RedisFixture",
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
					PkgPath:        "github.com/example/shared",
					TransferFields: []string{"DSN"},
				},
				{
					Identifier:     "RedisFixture",
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
					PkgPath:        "github.com/example/fixtures",
					TransferFields: []string{"ConnStr"},
				},
			}

			src, err := gotestgen.GenerateSharedSetup(fixtures)
			gotest.NoError(it, err)

			gotest.MatchSnapshot(it, string(src))
		})
	})

	t.When("default timeout", func(w *gotest.T) {
		w.It("uses default fixture config", func(it *gotest.T) {
			fixtures := []gotestgen.SharedFixtureInfo{
				{
					Identifier:     "PGFixture",
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

			gotest.NotContains(it, code, "SharedFixtureConfig()",
				"should not call SharedFixtureConfig when HasConfig is false")
		})
	})

	t.When("with config marker", func(w *gotest.T) {
		w.It("uses the marker config for the timeout", func(it *gotest.T) {
			fixtures := []gotestgen.SharedFixtureInfo{
				{
					Identifier:     "PGFixture",
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

			gotest.MatchSnapshot(it, code)
		})
	})

	t.When("teardown budget", func(w *gotest.T) {
		w.It("floors an undeclared timeout instead of reporting a bare 30s", func(it *gotest.T) {
			fixtures := []gotestgen.SharedFixtureInfo{
				{
					Identifier:     "PGFixture",
					PkgPath:        "github.com/example/fixtures",
					HasConfig:      true,
					TransferFields: []string{"ConnStr"},
				},
			}

			src, err := gotestgen.GenerateSharedSetup(fixtures)
			gotest.NoError(it, err)

			code := string(src)

			// A declared Timeout of 0 means "no deadline", not "takes no time".
			// Feeding it straight into ƒmaxTimeout would report a flat 30s budget
			// and let the supervisor force-kill a teardown still releasing
			// resources — matching gotestruntime.supervisorBudget keeps the three
			// budget sites agreeing.
			gotest.Contains(it, code, "func ƒsupervisorBudget(timeout time.Duration) time.Duration",
				"the generated program must floor an unbounded fixture's budget")
			gotest.Contains(it, code, "return gotest.DefaultFixtureConfig().Timeout",
				"the floor is the same default the in-process DAG uses")
			gotest.Contains(it, code, "if ƒb := ƒsupervisorBudget(ƒcfg_sf0.Timeout); ƒb > ƒmaxTimeout {",
				"the reported budget must go through the floor")
			gotest.NotContains(it, code, "if ƒcfg_sf0.Timeout > ƒmaxTimeout {",
				"the raw declared timeout must not reach the budget calculation")
		})
	})

	t.When("retry logic", func(w *gotest.T) {
		w.It("generates retry loop with delay", func(it *gotest.T) {
			fixtures := []gotestgen.SharedFixtureInfo{
				{
					Identifier:     "PGFixture",
					PkgPath:        "github.com/example/fixtures",
					TransferFields: []string{"ConnStr"},
				},
			}

			src, err := gotestgen.GenerateSharedSetup(fixtures)
			gotest.NoError(it, err)

			gotest.MatchSnapshot(it, string(src))
		})
	})

	t.When("state key format", func(w *gotest.T) {
		w.It("uses fully qualified package path and identifier", func(it *gotest.T) {
			fixtures := []gotestgen.SharedFixtureInfo{
				{
					Identifier:     "PGFixture",
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
					PkgPath:        "github.com/example/fixtures",
					TransferFields: []string{"ConnStr"},
				},
			}

			src, err := gotestgen.GenerateSharedSetup(fixtures)
			gotest.NoError(it, err)

			gotest.MatchSnapshot(it, string(src))
		})
	})

	t.When("reverse teardown on error", func(w *gotest.T) {
		w.It("tears down sf0 in reverse order", func(it *gotest.T) {
			fixtures := []gotestgen.SharedFixtureInfo{
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
					PkgPath:        "github.com/example/project/tests/fixtures",
					TransferFields: []string{"DSN"},
				},
				{
					Identifier:     "GCSFixture",
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
