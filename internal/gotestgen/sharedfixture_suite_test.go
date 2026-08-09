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

			// A bare marker call at the top of main would panic before the
			// handshake, killing the process attributed to nothing.
			gotest.Contains(it, code, `gotestruntime.DeriveFixtureConfig("PGFixture", sf0.SharedFixtureConfig)`,
				"the config marker must run inside the containment frame")
			gotest.NotContains(it, code, "ƒcfg_sf0 := sf0.SharedFixtureConfig()",
				"the marker must not be called outside the containment frame")

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

			// A declared Timeout of 0 means "no deadline", not "takes no time",
			// so each member's contribution goes through the shared floor. And
			// because the generated teardown loop is strictly sequential, the
			// budget is the SUM of the members' floored budgets — a max would
			// force-kill a fixture set whose teardowns each obeyed their own
			// declared Timeout.
			gotest.Contains(it, code, "ƒbudget += gotestruntime.SupervisorBudget(ƒcfg_sf0.Timeout)",
				"each member's budget must go through the shared floor and into the sum")
			gotest.NotContains(it, code, "func ƒsupervisorBudget(",
				"no local copy of the floor may remain")
			gotest.NotContains(it, code, "ƒb > ƒmaxTimeout",
				"a max would under-budget a sequential teardown chain")
		})

		w.It("reports the budget on the failure handshake too", func(it *gotest.T) {
			fixtures := []gotestgen.SharedFixtureInfo{
				{
					Identifier:     "PGFixture",
					PkgPath:        "github.com/example/fixtures",
					TransferFields: []string{"ConnStr"},
				},
			}

			src, err := gotestgen.GenerateSharedSetup(fixtures)
			gotest.NoError(it, err)

			// A failed setup still has succeeded siblings to release, and the
			// runner sizes its wait from this field; an error line without it
			// left the runner guessing with a flat 30s.
			gotest.Contains(it, string(src),
				`"{\"key\":\"_done\",\"error\":\"one or more shared fixtures failed\",\"teardownBudget\":%s}\n"`,
				"the failure handshake must carry the teardown budget")
		})
	})

	t.When("shutdown protocol", func(w *gotest.T) {
		w.It("reports first and waits for the runner's signal even on failure", func(it *gotest.T) {
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

			// The runner owns shutdown timing — only it knows when every suite
			// has stopped using the fixtures. A subprocess that tore down on its
			// own after a setup failure raced the suites of its healthy siblings.
			gotest.Contains(it, code, "ƒctx, ƒstop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)",
				"the process must be shutdown-capable from birth")
			gotest.Contains(it, code, "<-ƒctx.Done()",
				"teardown must wait for the runner's signal")
			gotest.Equal(it, 1, strings.Count(code, "RunFixtureTeardown"),
				"one teardown epilogue — a separate failure-path copy tears down behind the runner's back")
			gotest.NotContains(it, code, "signal.Notify(sig",
				"the late signal registration left a window where SIGTERM killed the process mid-setup")
		})
	})

	t.When("teardown policy", func(w *gotest.T) {
		w.It("delegates teardown to the same policy the in-process DAG runs", func(it *gotest.T) {
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

			// The inlined ƒteardown was the copy that never learned to hold an
			// AfterAll to its declared Timeout — the DAG side gained the
			// verdict while the subprocess kept context-bounding alone.
			gotest.Contains(it, code, "gotestruntime.RunFixtureTeardown(context.Background(), gotestruntime.FixtureTeardown{",
				"teardown must go through the shared policy")
			gotest.NotContains(it, code, "func ƒteardown(",
				"no second copy of the teardown policy may remain")
			gotest.Contains(it, code, "Budget:   ƒcfg_sf0.Timeout,",
				"a declared Timeout must become a teardown verdict")
			gotest.Contains(it, code, "if ƒerrs[0] == nil || errors.Is(ƒerrs[0], gotestruntime.ErrSetupOverran) {",
				"a setup that overran its budget still initialized — its resources exist and must be released")
		})

		w.It("gives an undeclared config no teardown verdict", func(it *gotest.T) {
			fixtures := []gotestgen.SharedFixtureInfo{
				{
					Identifier:     "PGFixture",
					PkgPath:        "github.com/example/fixtures",
					TransferFields: []string{"ConnStr"},
				},
			}

			src, err := gotestgen.GenerateSharedSetup(fixtures)
			gotest.NoError(it, err)

			// A fixture with no config of its own declared no deadline, so it
			// gets no overrun verdict — only the context bound the default gives.
			gotest.Contains(it, string(src), "Budget:   0,",
				"an undeclared Timeout must not become a teardown verdict")
		})
	})

	t.When("retry logic", func(w *gotest.T) {
		w.It("delegates setup to the same policy the in-process DAG runs", func(it *gotest.T) {
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

			// An inlined second copy of the loop is how the subprocess ended up
			// without panic containment, retry-on-panic or a Timeout verdict
			// while the DAG had all three.
			gotest.Contains(it, code, "gotestruntime.RunFixtureSetup(ƒctx, gotestruntime.FixtureSetup{",
				"setup must go through the shared policy")
			gotest.NotContains(it, code, "ƒattempts := 1 + ƒcfg_sf0.Retries",
				"no second copy of the retry loop may remain")

			// A fixture with no config of its own declared no deadline, so it
			// gets no overrun verdict — only the context bound the default gives.
			gotest.Contains(it, code, "Budget:     0,",
				"an undeclared Timeout must not become a verdict")

			gotest.MatchSnapshot(it, code)
		})
	})

	t.When("imports", func(w *gotest.T) {
		w.It("declares exactly what the generated program still uses", func(it *gotest.T) {
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
			gotest.Equal(it, []string{
				"context",
				"encoding/json",
				"errors",
				"fmt",
				"os",
				"os/signal",
				"sync",
				"syscall",
				"time",
				"github.com/mvrahden/go-test/pkg/gotest",
				"github.com/mvrahden/go-test/pkg/gotestruntime",
				"github.com/example/fixtures",
			}, paths)

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

	t.When("determinism", func(w *gotest.T) {
		w.It("generates byte-identical output on repeated runs", func(it *gotest.T) {
			// Two parent assignments is the smallest input where Go map
			// iteration order could shuffle the emitted statements.
			fixtures := []gotestgen.SharedFixtureInfo{
				{
					Identifier:     "AlphaSharedFixture",
					PkgPath:        "github.com/example/fixtures",
					TransferFields: []string{"A"},
				},
				{
					Identifier:     "BetaSharedFixture",
					PkgPath:        "github.com/example/fixtures",
					TransferFields: []string{"B"},
				},
				{
					Identifier: "GammaSharedFixture",
					PkgPath:    "github.com/example/fixtures",
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
