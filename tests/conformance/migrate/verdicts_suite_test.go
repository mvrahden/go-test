package migrate_test

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/gotestcli"
)

// MigrateVerdictsTestSuite: migrate translates only where gotest keeps
// testify's verdict, and marks where it cannot.
type MigrateVerdictsTestSuite struct {
	Mod *gotestcli.TestifyModuleSharedFixture
}

func (s *MigrateVerdictsTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

// suiteFile is a testify test file of package p: its suite VSuite holds
// fields, then body.
func suiteFile(imports, fields, body string) string {
	return "package p\n\nimport (\n\t\"testing\"\n" + imports + "\n\t\"github.com/stretchr/testify/suite\"\n)\n\n" +
		"type item struct{ n int }\n\ntype VSuite struct {\n\tsuite.Suite\n" + fields + "}\n\n" + body +
		"\nfunc TestV(t *testing.T) { suite.Run(t, new(VSuite)) }\n"
}

// migrated migrates files in a fresh module and returns migrate's exit
// code and the migrated p/a_test.go.
func (s *MigrateVerdictsTestSuite) migrated(t *gotest.T, files map[string]string) (int, string) {
	dir := t.TempDir()
	gotest.NoError(t, s.Mod.Write(dir, files))
	mig, err := testkit.RunCLIWithEnv(t.Context(), dir, moduleEnv, "migrate", "./...")
	gotest.NoError(t, err)
	return mig.Exit, readFile(t, dir, "p/a_test.go")
}

func (s *MigrateVerdictsTestSuite) TestAnAssertionOnAGoroutine(t *gotest.T) {
	t.When("an assertion runs on a goroutine a go statement starts", func(w *gotest.T) {
		w.It("migrate leaves it unconverted under a marker naming gotest.Go", func(it *gotest.T) {
			exit, out := s.migrated(it, map[string]string{"p/a_test.go": suiteFile("", "", `func (s *VSuite) TestA() {
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.Equal(1, 1)
	}()
	<-done
}
`)})
			gotest.Equal(it, 1, exit, out)
			gotest.Regexp(it, `// TODO\(gotest-migrate\): assertion Equal runs on a goroutine [^\n]*gotest\.Go[^\n]*\n\t\ts\.Equal\(1, 1\)\n`, out)
		})
	})
}

func (s *MigrateVerdictsTestSuite) TestElementsComparedDeeply(t *gotest.T) {
	t.When("ElementsMatch or Subset compares pointer or interface elements", func(w *gotest.T) {
		w.It("migrate leaves it unconverted under a marker saying gotest compares them with ==", func(it *gotest.T) {
			exit, out := s.migrated(it, map[string]string{"p/a_test.go": suiteFile("", "", `func (s *VSuite) TestA() {
	s.ElementsMatch([]*item{{1}}, []*item{{1}})
	s.Subset([]any{1, 2}, []any{1})
}
`)})
			gotest.Equal(it, 1, exit, out)
			gotest.Regexp(it, `// TODO\(gotest-migrate\): testify ElementsMatch compared the \*item elements deeply; gotest\.ElementsMatch compares them with ==[^\n]*\n\ts\.ElementsMatch\(`, out)
			gotest.Regexp(it, `// TODO\(gotest-migrate\): testify Subset compared the any elements deeply; gotest\.Subset compares them with ==[^\n]*\n\ts\.Subset\(`, out)
		})
	})
}

func (s *MigrateVerdictsTestSuite) TestZeroOrEmptyOnAnInterface(t *gotest.T) {
	t.When("Zero or Empty checks a value of an interface type", func(w *gotest.T) {
		w.It("migrate leaves it unconverted under a marker saying testify judged the value inside", func(it *gotest.T) {
			exit, out := s.migrated(it, map[string]string{"p/a_test.go": suiteFile("", "", `func (s *VSuite) TestA() {
	var err error
	s.Zero(err)
	var v any
	s.Empty(v)
}
`)})
			gotest.Equal(it, 1, exit, out)
			gotest.Regexp(it, `// TODO\(gotest-migrate\): testify Zero on interface type error also passes for a zero value inside it[^\n]*\n\ts\.Zero\(err\)\n`, out)
			gotest.Regexp(it, `// TODO\(gotest-migrate\): testify Empty on interface type any judges the value inside it[^\n]*\n\ts\.Empty\(v\)\n`, out)
		})
	})
}

func (s *MigrateVerdictsTestSuite) TestAnUntypedConstantOfAnotherType(t *gotest.T) {
	t.When("Equal compares an untyped constant with a value of another type", func(w *gotest.T) {
		w.It("migrate leaves it unconverted under a marker naming both types", func(it *gotest.T) {
			exit, out := s.migrated(it, map[string]string{"p/a_test.go": suiteFile("", "", `func (s *VSuite) TestA() {
	var x int64 = 5
	s.NotEqual(5, x)
}
`)})
			gotest.Equal(it, 1, exit, out)
			gotest.Regexp(it, `// TODO\(gotest-migrate\): testify NotEqual compared 5 as int with a value of type int64[^\n]*\n\ts\.NotEqual\(5, x\)\n`, out)
		})
	})
}

func (s *MigrateVerdictsTestSuite) TestTearDownSuiteAfterASetupSuiteThatCanStop(t *gotest.T) {
	t.When("a suite declares TearDownSuite and a SetupSuite that can stop early", func(w *gotest.T) {
		w.It("migrate marks AfterAll, which gotest runs even after BeforeAll stopped", func(it *gotest.T) {
			exit, out := s.migrated(it, map[string]string{"p/a_test.go": suiteFile("", "\tdb *item\n", `func (s *VSuite) SetupSuite() {
	s.Require().NotZero(1)
	s.db = &item{1}
}

func (s *VSuite) TearDownSuite() { s.db.n = 0 }

func (s *VSuite) TestA() {}
`)})
			gotest.Equal(it, 1, exit, out)
			gotest.Regexp(it, `// TODO\(gotest-migrate\): testify skipped TearDownSuite when SetupSuite stopped early[^\n]*gotest runs AfterAll even then[^\n]*\nfunc \(s \*VTestSuite\) AfterAll\(`, out)
		})
	})
}

func (s *MigrateVerdictsTestSuite) TestParallelAndCleanupOnTheSuitesT(t *gotest.T) {
	t.When("a test calls s.T().Parallel() or s.T().Cleanup", func(w *gotest.T) {
		w.It("migrate marks each call, naming SuiteConfig.Parallel, AfterEach and AfterAll", func(it *gotest.T) {
			exit, out := s.migrated(it, map[string]string{"p/a_test.go": suiteFile("", "", `func (s *VSuite) TestA() {
	s.T().Parallel()
	s.T().Cleanup(func() {})
}
`)})
			gotest.Equal(it, 1, exit, out)
			gotest.Regexp(it, `// TODO\(gotest-migrate\): t\.T\(\)\.Parallel\(\) in a gotest suite — set Parallel in the suite's SuiteConfig[^\n]*\n\tt\.T\(\)\.Parallel\(\)\n`, out)
			gotest.Regexp(it, `// TODO\(gotest-migrate\): t\.T\(\)\.Cleanup in a gotest suite — release the resource in AfterEach, or in AfterAll[^\n]*\n\tt\.T\(\)\.Cleanup\(`, out)
		})
	})
}

func (s *MigrateVerdictsTestSuite) TestTestsInAnotherOrder(t *gotest.T) {
	t.When("gotest would run the tests of a suite with fields in another order than testify", func(w *gotest.T) {
		w.It("migrate notes on the suite that the order changed", func(it *gotest.T) {
			exit, out := s.migrated(it, map[string]string{"p/a_test.go": suiteFile("", "\tn int\n", `func (s *VSuite) TestB() { s.n++ }

func (s *VSuite) TestA() { s.Equal(0, s.n) }
`)})
			gotest.Equal(it, 1, exit, out)
			gotest.Regexp(it, `// TODO\(gotest-migrate\): gotest runs VTestSuite's tests in source order, testify ran them in name order: TestB now runs before TestA[^\n]*\ntype VTestSuite struct`, out)
		})
	})
}

func (s *MigrateVerdictsTestSuite) TestABasesTestsAmongTheChilds(t *gotest.T) {
	t.When("a base declares a test whose name sorts between the child's own tests", func(w *gotest.T) {
		w.It("the migrated child declares its forwarding method between them", func(it *gotest.T) {
			exit, out := s.migrated(it, map[string]string{"p/a_test.go": `package p

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type BaseSuite struct {
	suite.Suite
	n int
}

func (b *BaseSuite) TestB() {}

type ChildSuite struct{ BaseSuite }

func (s *ChildSuite) TestA() {}

func (s *ChildSuite) TestC() {}

func TestChild(t *testing.T) { suite.Run(t, new(ChildSuite)) }
`})
			gotest.Equal(it, 0, exit, out)
			gotest.Contains(it, out, "func (s *ChildTestSuite) TestA(t *gotest.T) {}\n\nfunc (s *ChildTestSuite) TestB(t *gotest.T) {\n\ts.base.TestB(t)\n}\n\nfunc (s *ChildTestSuite) TestC(t *gotest.T) {}\n")
		})
	})
}

func (s *MigrateVerdictsTestSuite) TestGOFLAGSAskingForModMod(t *gotest.T) {
	t.When("GOFLAGS asks for -mod=mod and go.sum lacks an entry the load needs", func(w *gotest.T) {
		w.It("migrate writes neither go.mod nor go.sum", func(it *gotest.T) {
			dir := it.TempDir()
			gotest.NoError(it, s.Mod.Write(dir, map[string]string{"p/a_test.go": suiteFile("", "", "func (s *VSuite) TestA() { s.True(true) }\n")}))
			var kept []string
			for line := range strings.Lines(s.Mod.GoSum) {
				if !strings.Contains(line, "go-difflib") {
					kept = append(kept, line)
				}
			}
			sum := strings.Join(kept, "")
			gotest.NoError(it, os.WriteFile(filepath.Join(dir, "go.sum"), []byte(sum), 0o600))
			mig, err := testkit.RunCLIWithEnv(it.Context(), dir, []string{"GOWORK=off", "GOFLAGS=-mod=mod"}, "migrate", "./...")
			gotest.NoError(it, err)
			gotest.Equal(it, sum, readFile(it, dir, "go.sum"), "stdout: %s\nstderr: %s", mig.Stdout, mig.Stderr)
			gotest.Equal(it, s.Mod.GoMod, readFile(it, dir, "go.mod"))
		})
	})
}

func (s *MigrateVerdictsTestSuite) TestTestifyImportsBetweenImportsThatStay(t *gotest.T) {
	t.When("testify imports sit between imports that stay", func(w *gotest.T) {
		w.It("the migrated import block holds no blank line where they stood", func(it *gotest.T) {
			exit, out := s.migrated(it, map[string]string{
				"x/x.go": "package x\n\nconst V = 1\n",
				"y/y.go": "package y\n\nconst V = 1\n",
				"p/a_test.go": `package p

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"os"
	"testing"

	"example.com/m/x"
	"github.com/stretchr/testify/suite"
	"example.com/m/y"
)

type ISuite struct{ suite.Suite }

func (s *ISuite) TestA() {
	require.True(s.T(), os.Getpid() > 0)
	s.Equal(x.V, y.V)
	fmt.Println()
}

func TestI(t *testing.T) { suite.Run(t, new(ISuite)) }
`,
			})
			gotest.Equal(it, 0, exit, out)
			gotest.Contains(it, out, "import (\n\t\"fmt\"\n\t\"os\"\n\n\t\"example.com/m/x\"\n\t\"example.com/m/y\"\n\t\"github.com/mvrahden/go-test/pkg/gotest\"\n)\n")
		})
	})
}
