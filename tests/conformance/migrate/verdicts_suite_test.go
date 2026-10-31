package migrate_test

import (
	"go/format"
	"os"
	"os/exec"
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

func (s *MigrateVerdictsTestSuite) TestASuiteWithNoTest(t *gotest.T) {
	t.When("a testify suite declares no test, of its own or from a base", func(w *gotest.T) {
		w.It("migrate leaves it unconverted under a marker saying gotest refuses a suite without a case", func(it *gotest.T) {
			src := suiteFile("", "", "func (s *VSuite) SetupSuite() {}\n")
			exit, out := s.migrated(it, map[string]string{"p/a_test.go": src})
			gotest.Equal(it, 1, exit, out)
			gotest.Regexp(it, `// TODO\(gotest-migrate\): VSuite declares no test; gotest refuses a suite without a Test, Benchmark or Fuzz method[^\n]*\ntype VSuite struct`, out)
			gotest.Equal(it, src, stripMarkers(out))
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

func (s *MigrateVerdictsTestSuite) TestNotNilOnAnInterface(t *gotest.T) {
	t.When("NotNil checks a value of an interface type", func(w *gotest.T) {
		w.It("migrate leaves it unconverted under a marker naming the typed nil and the non-nilable value inside", func(it *gotest.T) {
			exit, out := s.migrated(it, map[string]string{"p/a_test.go": suiteFile("", "", `func (s *VSuite) TestA() {
	var err error
	s.NotNil(err)
	var v any = 5
	s.Require().NotNil(v)
	p := &item{}
	s.NotNil(p)
}
`)})
			gotest.Equal(it, 1, exit, out)
			gotest.Regexp(it, `// TODO\(gotest-migrate\): testify NotNil on interface type error fails a typed nil inside it, which gotest\.NotZero passes, and passes a non-nilable value inside it, which gotest\.NotNil fails[^\n]*\n\ts\.NotNil\(err\)\n`, out)
			gotest.Regexp(it, `// TODO\(gotest-migrate\): testify NotNil on interface type any fails a typed nil inside it[^\n]*\n\ts\.Require\(\)\.NotNil\(v\)\n`, out)
			gotest.Contains(it, out, "\tgotest.NotNil(t, p)\n")
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

func (s *MigrateVerdictsTestSuite) TestAnUntypedConstantOfAnotherPackage(t *gotest.T) {
	t.When("Equal or NotEqual compares an untyped constant another package declares with a value of another type", func(w *gotest.T) {
		w.It("migrate leaves it unconverted under a marker naming both types", func(it *gotest.T) {
			exit, out := s.migrated(it, map[string]string{
				"x/x.go": "package x\n\nconst Port = 8080\n",
				"p/a_test.go": suiteFile("\t\"example.com/m/x\"", "", `func (s *VSuite) TestA() {
	var port uint16 = 8080
	s.NotEqual(x.Port, port)
}
`)})
			gotest.Equal(it, 1, exit, out)
			gotest.Regexp(it, `// TODO\(gotest-migrate\): testify NotEqual compared x\.Port as int with a value of type uint16[^\n]*\n\ts\.NotEqual\(x\.Port, port\)\n`, out)
		})
	})
}

func (s *MigrateVerdictsTestSuite) TestTwoConcreteTypes(t *gotest.T) {
	t.When("Equal or NotEqual compares values of two concrete types, one assignable to the other", func(w *gotest.T) {
		exit, out := s.migrated(w, map[string]string{"p/a_test.go": suiteFile("", "", `type IDs []int

type failure struct{}

func (*failure) Error() string { return "" }

func (s *VSuite) TestA() {
	ids := IDs{1}
	s.NotEqual([]int{1}, ids)
	var err error
	s.Equal(err, err)
	s.NotEqual(err, &failure{})
}
`)})
		w.It("migrate leaves it unconverted under a marker naming both types", func(it *gotest.T) {
			gotest.Equal(it, 1, exit, out)
			gotest.Regexp(it, `// TODO\(gotest-migrate\): testify NotEqual compared a \[\]int with a IDs, types it never finds equal[^\n]*\n\ts\.NotEqual\(\[\]int\{1\}, ids\)\n`, out)
		})
		w.It("an interface compared with a value it holds is translated with Equal instantiated with the interface", func(it *gotest.T) {
			gotest.Contains(it, out, "\tgotest.NotEqual[error](t, err, &failure{})\n")
		})
	})
}

func (s *MigrateVerdictsTestSuite) TestLenOnAPointerToAnArray(t *gotest.T) {
	t.When("Len checks a pointer to an array", func(w *gotest.T) {
		w.It("migrate leaves it unconverted under a marker naming len", func(it *gotest.T) {
			exit, out := s.migrated(it, map[string]string{"p/a_test.go": suiteFile("", "", `func (s *VSuite) TestA() {
	var p *[3]int
	s.Len(p, 3)
}
`)})
			gotest.Equal(it, 1, exit, out)
			gotest.Regexp(it, `// TODO\(gotest-migrate\): testify Len on \*\[3\]int counted the array's length[^\n]*len\(p\)[^\n]*\n\ts\.Len\(p, 3\)\n`, out)
		})
	})
}

func (s *MigrateVerdictsTestSuite) TestInDeltaWhereANaNDecides(t *gotest.T) {
	t.When("InDelta compares two values that may both be NaN, or takes a delta that may be NaN", func(w *gotest.T) {
		w.It("migrate leaves it unconverted under a marker naming the NaN", func(it *gotest.T) {
			exit, out := s.migrated(it, map[string]string{"p/a_test.go": suiteFile("", "", `func (s *VSuite) TestA() {
	want, got, eps := 1.0, 1.0, 0.1
	s.InDelta(want, got, 0.1)
	s.InDelta(1.0, got, eps)
}
`)})
			gotest.Equal(it, 1, exit, out)
			gotest.Regexp(it, `// TODO\(gotest-migrate\): testify InDelta passed when both values were NaN[^\n]*\n\ts\.InDelta\(want, got, 0\.1\)\n`, out)
			gotest.Regexp(it, `// TODO\(gotest-migrate\): testify InDelta passed any difference within a NaN delta[^\n]*\n\ts\.InDelta\(1\.0, got, eps\)\n`, out)
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

// teardownMarker matches the marker above a migrated AfterAll that gotest
// runs even after BeforeAll stopped.
const teardownMarker = `// TODO\(gotest-migrate\): testify skipped TearDownSuite when SetupSuite stopped early[^\n]*gotest runs AfterAll even then[^\n]*\nfunc \(s \*VTestSuite\) AfterAll\(`

func (s *MigrateVerdictsTestSuite) TestASetupSuiteThatStopsThroughWhatItCalls(t *gotest.T) {
	t.When("a suite declares TearDownSuite and a SetupSuite that calls a function of the migrated files that can stop early, or hands its T or the suite to code declared elsewhere", func(w *gotest.T) {
		w.It("migrate marks AfterAll, which gotest runs even after BeforeAll stopped", func(it *gotest.T) {
			for _, setup := range []string{
				"requireDocker(s.T())",
				"s.open()",
				"x.Check(s.T())",
			} {
				exit, out := s.migrated(it, map[string]string{
					"x/x.go": "package x\n\nimport \"testing\"\n\nfunc Check(t *testing.T) { t.Helper() }\n",
					"p/a_test.go": suiteFile("\t\"example.com/m/x\"", "\tdb *item\n", `func requireDocker(t *testing.T) {
	if t != nil {
		t.Skip("no docker")
	}
}

func (s *VSuite) open() { s.Require().NotZero(1) }

func (s *VSuite) SetupSuite() {
	`+setup+`
	s.db = &item{1}
}

func (s *VSuite) TearDownSuite() { s.db.n = 0 }

func (s *VSuite) TestA() { _ = x.Check }
`)})
				gotest.Equal(it, 1, exit, setup+"\n"+out)
				gotest.Regexp(it, teardownMarker, out, setup)
			}
		})
	})
}

func (s *MigrateVerdictsTestSuite) TestSuiteHooksABaseGives(t *gotest.T) {
	t.When("a base suite gives a child the SetupSuite that can stop early or the TearDownSuite", func(w *gotest.T) {
		w.It("migrate marks the AfterAll the child declares, or notes on the child that the AfterAll it forwards runs even after BeforeAll stopped", func(it *gotest.T) {
			exit, out := s.migrated(it, map[string]string{"p/a_test.go": suiteFile("", "\tdb *item\n", `type BaseSuite struct{ suite.Suite }

func (b *BaseSuite) SetupSuite() { b.Require().NotZero(1) }

type ChildSuite struct {
	BaseSuite
	db *item
}

func (s *ChildSuite) TearDownSuite() { s.db.n = 0 }

func (s *ChildSuite) TestA() {}

func TestChild(t *testing.T) { suite.Run(t, new(ChildSuite)) }

func (s *VSuite) SetupSuite() { s.Require().NotZero(1) }

func (s *VSuite) TestA() {}
`)})
			gotest.Equal(it, 1, exit, out)
			gotest.Regexp(it, `// TODO\(gotest-migrate\): testify skipped TearDownSuite when SetupSuite stopped early[^\n]*\nfunc \(s \*ChildTestSuite\) AfterAll\(`, out)

			exit, out = s.migrated(it, map[string]string{"p/a_test.go": `package p

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type item struct{ n int }

type BaseSuite struct {
	suite.Suite
	db *item
}

func (b *BaseSuite) TearDownSuite() { b.db.n = 0 }

type ChildSuite struct{ BaseSuite }

func (s *ChildSuite) SetupSuite() {
	s.Require().NotZero(1)
	s.db = &item{1}
}

func (s *ChildSuite) TestA() {}

func TestChild(t *testing.T) { suite.Run(t, new(ChildSuite)) }
`})
			gotest.Equal(it, 1, exit, out)
			gotest.Regexp(it, `// TODO\(gotest-migrate\): testify skipped the TearDownSuite ChildTestSuite has from its base field when SetupSuite stopped early[^\n]*gotest runs the AfterAll forwarding it even then[^\n]*\ntype ChildTestSuite struct`, out)
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

// vendoredFiles is a testify suite and a test file that keeps the gotest
// import, so vendoring copies what the migrated suite needs.
func vendoredFiles() map[string]string {
	return map[string]string{
		"p/a_test.go":    suiteFile("", "", "func (s *VSuite) TestA() { s.True(true) }\n"),
		"p/keep_test.go": "package p\n\nimport _ \"github.com/mvrahden/go-test/pkg/gotest\"\n",
	}
}

// goIn runs the go command in dir under env.
func goIn(t *gotest.T, dir string, env []string, args ...string) {
	cmd := exec.CommandContext(t.Context(), "go", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	gotest.NoError(t, err, "go %s: %s", strings.Join(args, " "), out)
}

// offline leaves the go command an empty module cache and no proxy, so
// only a vendor directory can serve the module's dependencies.
func offline(t *gotest.T) []string {
	return []string{"GOFLAGS=", "GOPROXY=off", "GOTOOLCHAIN=local", "GOMODCACHE=" + t.TempDir()}
}

func (s *MigrateVerdictsTestSuite) TestAVendoredWorkspace(t *gotest.T) {
	t.When("a workspace is reached through a symlinked directory", func(w *gotest.T) {
		w.It("migrate matches the files the go command names through the symlink", func(it *gotest.T) {
			real := it.TempDir()
			root := filepath.Join(it.TempDir(), "link")
			if err := os.Symlink(real, root); err != nil {
				it.Skipf("symlink: %v", err)
			}
			dir := filepath.Join(root, "m")
			gotest.NoError(it, s.Mod.Write(dir, vendoredFiles()))
			gotest.NoError(it, os.WriteFile(filepath.Join(root, "go.work"), []byte("go 1.26.0\n\nuse ./m\n"), 0o600))
			work := "GOWORK=" + filepath.Join(root, "go.work")
			mig, err := testkit.RunCLIWithEnv(it.Context(), dir, []string{work}, "migrate", "./...")
			gotest.NoError(it, err)
			gotest.Equal(it, 0, mig.Exit, "stdout: %s\nstderr: %s", mig.Stdout, mig.Stderr)
			gotest.Contains(it, readFile(it, dir, "p/a_test.go"), "gotest.True(t, true)")
		})
	})
	t.When("a workspace vendors its modules and the module cache is empty", func(w *gotest.T) {
		w.It("migrate loads from the workspace's vendor directory and converts the suite", func(it *gotest.T) {
			root := it.TempDir()
			dir := filepath.Join(root, "m")
			gotest.NoError(it, s.Mod.Write(dir, vendoredFiles()))
			gotest.NoError(it, os.WriteFile(filepath.Join(root, "go.work"), []byte("go 1.26.0\n\nuse ./m\n"), 0o600))
			work := "GOWORK=" + filepath.Join(root, "go.work")
			goIn(it, root, []string{work, "GOFLAGS="}, "work", "vendor")
			mig, err := testkit.RunCLIWithEnv(it.Context(), dir, append(offline(it), work), "migrate", "./...")
			gotest.NoError(it, err)
			gotest.Equal(it, 0, mig.Exit, "stdout: %s\nstderr: %s", mig.Stdout, mig.Stderr)
			gotest.Contains(it, readFile(it, dir, "p/a_test.go"), "gotest.True(t, true)")
		})
	})
}

func (s *MigrateVerdictsTestSuite) TestAWorkspaceTurnedOffByGoEnv(t *gotest.T) {
	t.When("go env -w GOWORK=off turns off a go.work above a vendored module", func(w *gotest.T) {
		w.It("migrate loads from the module's vendor directory and converts the suite", func(it *gotest.T) {
			root := it.TempDir()
			dir := filepath.Join(root, "m")
			gotest.NoError(it, s.Mod.Write(dir, vendoredFiles()))
			gotest.NoError(it, os.WriteFile(filepath.Join(root, "go.work"), []byte("go 1.26.0\n\nuse ./m\n"), 0o600))
			goIn(it, dir, []string{"GOWORK=off", "GOFLAGS="}, "mod", "vendor")
			goenv := filepath.Join(it.TempDir(), "env")
			gotest.NoError(it, os.WriteFile(goenv, []byte("GOWORK=off\n"), 0o600))
			mig, err := testkit.RunCLIWithEnv(it.Context(), dir, append(offline(it), "GOENV="+goenv, "GOWORK="), "migrate", "./...")
			gotest.NoError(it, err)
			gotest.Equal(it, 0, mig.Exit, "stdout: %s\nstderr: %s", mig.Stdout, mig.Stderr)
			gotest.Contains(it, readFile(it, dir, "p/a_test.go"), "gotest.True(t, true)")
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

func (s *MigrateVerdictsTestSuite) TestACommentOnARemovedImport(t *gotest.T) {
	t.When("an import migrate removes carries a comment on its line or above it", func(w *gotest.T) {
		w.It("the comment goes with the import and the file stays as gofmt formats it", func(it *gotest.T) {
			files := map[string]string{
				"p/a_test.go": `package p

import (
	"fmt" // printing
	"testing" // the runner

	// assertions
	"github.com/stretchr/testify/suite" // suites
	"strings"
)

type ASuite struct{ suite.Suite }

func (s *ASuite) TestA() { s.Equal("a", strings.ToLower("A")); fmt.Println() }

func TestA(t *testing.T) { suite.Run(t, new(ASuite)) }
`,
				"p/b_test.go": `package p

import "testing" // the runner
import "github.com/stretchr/testify/suite" // suites
import "fmt" // printing

type BSuite struct{ suite.Suite }

func (s *BSuite) TestB() { s.Equal(1, 1); fmt.Println() }

func TestB(t *testing.T) { suite.Run(t, new(BSuite)) }
`,
			}
			dir := it.TempDir()
			gotest.NoError(it, s.Mod.Write(dir, files))
			mig, err := testkit.RunCLIWithEnv(it.Context(), dir, moduleEnv, "migrate", "./...")
			gotest.NoError(it, err)
			gotest.Equal(it, 0, mig.Exit, "stdout: %s\nstderr: %s", mig.Stdout, mig.Stderr)
			a, b := readFile(it, dir, "p/a_test.go"), readFile(it, dir, "p/b_test.go")
			gotest.Contains(it, a, "import (\n\t\"fmt\" // printing\n\n\t\"strings\"\n\n\t\"github.com/mvrahden/go-test/pkg/gotest\"\n)\n\ntype ATestSuite")
			gotest.Contains(it, b, "package p\n\nimport (\n\t\"fmt\" // printing\n\n\t\"github.com/mvrahden/go-test/pkg/gotest\"\n)\n\ntype BTestSuite")
			for name, src := range map[string]string{"a": a, "b": b} {
				formatted, err := format.Source([]byte(src))
				gotest.NoError(it, err)
				gotest.Equal(it, string(formatted), src, name)
			}
		})
	})
}
