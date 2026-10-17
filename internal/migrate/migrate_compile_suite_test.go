package migrate_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/mvrahden/go-test/internal/migrate"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/gotestcli"
)

// MigrateCompileTestSuite migrates consumer modules end to end: the result
// must compile, every error it still has must carry a marker on its line,
// and the suites must run with their hooks.
type MigrateCompileTestSuite struct {
	Mod *gotestcli.TestifyModuleSharedFixture
	CLI *gotestcli.BinarySharedFixture
}

func (s *MigrateCompileTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *MigrateCompileTestSuite) migrate(t *gotest.T, files map[string]string, patterns ...string) pkgRun {
	if len(patterns) == 0 {
		patterns = []string{"..."}
	}
	return migrateModule(t, s.Mod, migrate.Options{}, files, patterns...)
}

// goCmd runs the go command in dir, stdout and stderr apart.
func goCmd(t *gotest.T, dir string, args ...string) (stdout, stderr string, err error) {
	cmd := exec.CommandContext(t.Context(), "go", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=")
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err = cmd.Run()
	return out.String(), errb.String(), err
}

// compileErr is "file:line:col: msg" in the go command's output.
var compileErr = regexp.MustCompile(`(?m)^(?:vet: )?(\S+\.go):(\d+):\d+: (.*)$`)

// unmarkedErrors compiles the test packages of dir with the build flags
// given and lists every compile error whose line has no marker above it:
// what migrate must never leave.
func unmarkedErrors(t *gotest.T, dir string, flags ...string) []string {
	args := append([]string{"test", "-count=1", "-run=^$"}, flags...)
	stdout, stderr, _ := goCmd(t, dir, append(args, "./...")...)
	var unmarked []string
	for _, m := range compileErr.FindAllStringSubmatch(stdout+stderr, -1) {
		path := m[1]
		if !filepath.IsAbs(path) {
			path = filepath.Join(dir, path)
		}
		src, err := os.ReadFile(path)
		gotest.NoError(t, err)
		lines := strings.Split(string(src), "\n")
		line, _ := strconv.Atoi(m[2])
		if line < 2 || line > len(lines) || !strings.Contains(lines[line-2], "TODO(gotest-migrate)") {
			unmarked = append(unmarked, m[0])
		}
	}
	return unmarked
}

// compiles reports the go command's verdict on dir's test packages.
func compiles(t *gotest.T, dir string, flags ...string) string {
	args := append([]string{"test", "-count=1", "-run=^$"}, flags...)
	stdout, stderr, err := goCmd(t, dir, append(args, "./...")...)
	if err != nil {
		return stdout + stderr
	}
	return ""
}

// runSuites runs gotest on patterns in dir.
func (s *MigrateCompileTestSuite) runSuites(t *gotest.T, dir string, args ...string) (stdout, stderr string, code int) {
	testkit.ScrubActionsEnv()
	cmd := exec.CommandContext(t.Context(), s.CLI.Binary, append([]string{"-v"}, args...)...) //nolint:gosec // G204: the CLI under test
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=", "PWD="+dir)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	_ = cmd.Run()
	return out.String(), errb.String(), cmd.ProcessState.ExitCode()
}

const tNamesSrc = `package p

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type NameSuite struct{ suite.Suite }

func (s *NameSuite) TestLocal() {
	t := s.T()
	require.True(t, true)
	s.Require().Equal(1, 1)
}

func (s *NameSuite) TestLoop() {
	for _, t := range []struct{ in, want int }{{1, 1}} {
		s.Require().Equal(t.want, t.in)
	}
}

type RecvSuite struct{ suite.Suite }

func (t *RecvSuite) TestA() { t.Require().True(true) }

func TestName(t *testing.T) { suite.Run(t, new(NameSuite)) }
func TestRecv(t *testing.T) { suite.Run(t, new(RecvSuite)) }
`

func (s *MigrateCompileTestSuite) TestTheTName(t *gotest.T) {
	run := s.migrate(t, map[string]string{"p/a_test.go": tNamesSrc})
	out := run.files["p/a_test.go"]

	t.It("compiles with no marker", func(it *gotest.T) {
		gotest.Zero(it, run.markers())
		gotest.Empty(it, compiles(it, run.dir))
	})

	t.When("the method already has a local t", func(w *gotest.T) {
		w.It("names the new parameter so the local keeps its *testing.T", func(it *gotest.T) {
			gotest.Contains(it, out, "func (s *NameTestSuite) TestLocal(gt *gotest.T) {\n\tt := gt.T()\n\trequire.True(t, true)\n\tgotest.Equal(gt, 1, 1)\n}")
		})
	})

	t.When("a loop variable is named t", func(w *gotest.T) {
		w.It("asserts against the parameter, not the loop variable", func(it *gotest.T) {
			gotest.Contains(it, out, "\t\tgotest.Equal(gt, t.want, t.in)\n")
		})
	})

	t.When("the receiver is named t", func(w *gotest.T) {
		w.It("gives the parameter another name", func(it *gotest.T) {
			gotest.Contains(it, out, "func (t *RecvTestSuite) TestA(gt *gotest.T) { gotest.True(gt, true) }")
		})
	})
}

const contractsSrc = `package p

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
)

type HookSuite struct{ suite.Suite }

var _ suite.SetupAllSuite = (*HookSuite)(nil)

var (
	_ suite.TearDownAllSuite = (*HookSuite)(nil)
	_                        = fmt.Sprint
)

func (s *HookSuite) SetupSuite()    { fmt.Println("HOOK SetupSuite") }
func (s *HookSuite) TearDownSuite() { fmt.Println("HOOK TearDownSuite") }
func (s *HookSuite) SetupTest()     { fmt.Println("HOOK SetupTest") }
func (s *HookSuite) TestA()         { fmt.Println("RAN TestA") }

func TestHook(t *testing.T) { suite.Run(t, new(HookSuite)) }
`

func (s *MigrateCompileTestSuite) TestInterfaceContracts(t *gotest.T) {
	run := s.migrate(t, map[string]string{"p/a_test.go": contractsSrc})
	out := run.files["p/a_test.go"]

	t.It("drops the testify contracts the suite no longer implements", func(it *gotest.T) {
		gotest.NotContains(it, out, "suite.SetupAllSuite")
		gotest.NotContains(it, out, "suite.TearDownAllSuite")
		gotest.Contains(it, out, "var (\n\t_ = fmt.Sprint\n)")
	})

	t.It("compiles with no marker", func(it *gotest.T) {
		gotest.Zero(it, run.markers())
		gotest.Empty(it, compiles(it, run.dir))
	})

	t.It("runs every hook", func(it *gotest.T) {
		stdout, stderr, code := s.runSuites(it, run.dir, "./p")
		gotest.Equal(it, 0, code, stdout+stderr)
		gotest.Regexp(it, `(?s)HOOK SetupSuite\n.*HOOK SetupTest\n.*RAN TestA\n.*HOOK TearDownSuite\n`, stdout)
	})
}

const callsSrc = `package p

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
)

func describe(ts suite.TestingSuite) string { return "ok" }

type CallSuite struct {
	suite.Suite
	n int
}

func (s *CallSuite) SetupTest() { s.n++ }

func (s *CallSuite) TestA() { fmt.Println("RAN TestA n =", s.n) }

func (s *CallSuite) TestB() {
	s.SetupTest()
	s.TestA()
}

func (s *CallSuite) TestValue() {
	f := s.TestA
	f()
}

func (s *CallSuite) TestIface() {
	s.Require().Equal("ok", describe(s))
}

func TestCall(t *testing.T) { suite.Run(t, new(CallSuite)) }
`

func (s *MigrateCompileTestSuite) TestMethodCalls(t *gotest.T) {
	run := s.migrate(t, map[string]string{"p/a_test.go": callsSrc})
	out := run.files["p/a_test.go"]

	t.When("a hook or test is called directly", func(w *gotest.T) {
		w.It("calls it under its gotest name, passing t", func(it *gotest.T) {
			gotest.Contains(it, out, "\ts.BeforeEach(t)\n\ts.TestA(t)\n")
		})
	})

	t.When("a test is taken as a method value", func(w *gotest.T) {
		w.It("marks it", func(it *gotest.T) {
			gotest.Contains(it, out, "\t// TODO(gotest-migrate): method value s.TestA: TestA now takes t *gotest.T, so it has a new type — call it with t instead\n\tf := s.TestA\n")
		})
	})

	t.When("the suite is passed as a suite.TestingSuite", func(w *gotest.T) {
		w.It("marks the call with the compile error", func(it *gotest.T) {
			gotest.Regexp(it, `// TODO\(gotest-migrate\): does not compile: cannot use s .* as suite\.TestingSuite value .*\n\tgotest\.Equal\(t, "ok", describe\(s\)\)`, out)
		})
	})

	t.It("leaves no compile error unmarked", func(it *gotest.T) {
		gotest.NotZero(it, run.markers())
		gotest.NotEmpty(it, compiles(it, run.dir))
		gotest.Empty(it, unmarkedErrors(it, run.dir))
	})
}

const typedSrc = `package p

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type ID [16]byte

type rec struct{ Tags []string }

type TypedSuite struct{ suite.Suite }

func (s *TypedSuite) TestTypes() {
	var xs []int
	var r rec
	var want any = "x"
	got := "x"
	s.Require().Zero(xs)
	s.Require().Zero(r)
	s.Require().NotZero(rec{Tags: []string{"a"}})
	s.Require().Empty(ID{})
	s.Require().NotEmpty(3)
	s.Require().Equal(want, got)
}

func (s *TypedSuite) TestArray() {
	s.Require().NotNil(ID{1})
}

func TestTyped(t *testing.T) { suite.Run(t, new(TypedSuite)) }
`

func (s *MigrateCompileTestSuite) TestTypedAssertions(t *gotest.T) {
	run := s.migrate(t, map[string]string{"p/a_test.go": typedSrc})
	out := run.files["p/a_test.go"]

	t.It("maps each zero or empty check to the gotest assertion that keeps its verdict", func(it *gotest.T) {
		gotest.Contains(it, out, "\tgotest.Nil(t, xs)\n")
		gotest.Contains(it, out, "\tgotest.Equal(t, rec{}, r)\n")
		gotest.Contains(it, out, "\tgotest.NotEqual(t, rec{}, rec{Tags: []string{\"a\"}})\n")
		gotest.Contains(it, out, "\tgotest.Zero(t, ID{})\n")
		gotest.Contains(it, out, "\tgotest.NotZero(t, 3)\n")
	})

	t.It("instantiates Equal where an interface meets a concrete value", func(it *gotest.T) {
		gotest.Contains(it, out, "\tgotest.Equal[any](t, want, got)\n")
	})

	t.It("marks NotNil on a type that cannot be nil", func(it *gotest.T) {
		gotest.Contains(it, out, "\t// TODO(gotest-migrate): NotNil on ID always passed in testify, as it can never be nil; gotest.NotNil fails on types that cannot be nil — delete it, or assert gotest.NotZero\n\ts.Require().NotNil(ID{1})\n")
		gotest.Equal(it, 1, run.markers())
	})

	t.It("keeps the verdicts: the converted test passes as it did", func(it *gotest.T) {
		path := filepath.Join(run.dir, "p", "a_test.go")
		fixed := regexp.MustCompile(`(?s)func \(s \*TypedTestSuite\) TestArray.*?\n}\n`).ReplaceAllString(out, "")
		gotest.NoError(it, os.WriteFile(path, []byte(fixed), 0o600))
		stdout, stderr, code := s.runSuites(it, run.dir, "./p")
		gotest.Equal(it, 0, code, stdout+stderr)
		gotest.Contains(it, stdout, "--- PASS: TestTypedTestSuite/TestTypes")
	})
}

const valueReadSrc = `package p

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
)

type ValSuite struct {
	suite.Suite
	n int
}

func (s ValSuite) TestReads() { fmt.Println("RAN TestReads"); s.Require().Zero(s.n) }

func TestVal(t *testing.T) { suite.Run(t, new(ValSuite)) }
`

func (s *MigrateCompileTestSuite) TestValueReceivers(t *gotest.T) {
	t.When("the method only reads the suite", func(w *gotest.T) {
		run := s.migrate(w, map[string]string{"p/a_test.go": valueReadSrc})

		w.It("gives it a pointer receiver, so gotest runs it", func(it *gotest.T) {
			gotest.Contains(it, run.files["p/a_test.go"], "func (s *ValTestSuite) TestReads(t *gotest.T) {")
			gotest.Zero(it, run.markers())
			stdout, stderr, code := s.runSuites(it, run.dir, "./p")
			gotest.Equal(it, 0, code, stdout+stderr)
			gotest.Contains(it, stdout, "RAN TestReads\n")
		})
	})

	t.When("the method changes the suite", func(w *gotest.T) {
		src := strings.Replace(valueReadSrc, `fmt.Println("RAN TestReads"); s.Require().Zero(s.n)`, "fmt.Println(s.n); s.n++", 1)
		run := s.migrate(w, map[string]string{"p/a_test.go": src})

		w.It("keeps the copy and marks it", func(it *gotest.T) {
			gotest.Contains(it, run.files["p/a_test.go"], "// TODO(gotest-migrate): TestReads has a value receiver: gotest calls only pointer-receiver methods, and testify ran this one on a copy it then dropped, so the changes it makes to the suite were lost — make the receiver *ValTestSuite if they should last\nfunc (s ValTestSuite) TestReads(t *gotest.T) {")
			gotest.Equal(it, 1, run.markers())
		})
	})
}

const gatedSuiteSrc = `package p

import (
	"fmt"

	"github.com/stretchr/testify/suite"
)

type DBSuite struct{ suite.Suite }

func (s *DBSuite) SetupSuite() { fmt.Println("HOOK DB SetupSuite") }
func (s *DBSuite) TestQuery()  { fmt.Println("RAN DB TestQuery") }
`

const gatedRunnerSrc = `//go:build integration

package p

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

func TestDB(t *testing.T) { suite.Run(t, new(DBSuite)) }
`

func (s *MigrateCompileTestSuite) TestGatedRunner(t *gotest.T) {
	t.When("the suite lives in one untagged file", func(w *gotest.T) {
		run := s.migrate(w, map[string]string{
			"p/suite_test.go":           gatedSuiteSrc,
			"p/run_integration_test.go": gatedRunnerSrc,
			"plain/plain_test.go":       "package plain\n",
		})

		w.It("moves the runner's constraint onto it", func(it *gotest.T) {
			gotest.Regexp(it, `^//go:build integration\n\npackage p\n`, run.files["p/suite_test.go"])
			gotest.Zero(it, run.markers())
		})

		w.It("runs the suite only with the tag, as before", func(it *gotest.T) {
			stdout, stderr, _ := s.runSuites(it, run.dir, "./...")
			gotest.NotContains(it, stdout, "HOOK DB SetupSuite", stderr)
			stdout, stderr, code := s.runSuites(it, run.dir, "-tags", "integration", "./p")
			gotest.Equal(it, 0, code, stdout+stderr)
			gotest.Contains(it, stdout, "HOOK DB SetupSuite\n")
			gotest.Contains(it, stdout, "RAN DB TestQuery\n")
		})
	})

	t.When("the suite spreads over two files", func(w *gotest.T) {
		run := s.migrate(w, map[string]string{
			"p/suite_test.go":           gatedSuiteSrc,
			"p/more_test.go":            "package p\n\nfunc (s *DBSuite) TestMore() {}\n",
			"p/run_integration_test.go": gatedRunnerSrc,
		})

		w.It("marks the suite instead of un-gating it silently", func(it *gotest.T) {
			gotest.Contains(it, run.files["p/suite_test.go"], "// TODO(gotest-migrate): DBSuite's runner in run_integration_test.go built only with //go:build integration; gotest runs a suite under its own file's constraint — move the suite and its methods into one file with that constraint\ntype DBTestSuite struct{}")
			gotest.Equal(it, 1, run.markers())
		})
	})
}

const mixinSrc = `package p

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
)

type dbMixin struct{ n int }

func (m *dbMixin) SetupTest()  { m.n = 1 }
func (m *dbMixin) TestShared() { fmt.Println("RAN TestShared") }

type MixSuite struct {
	suite.Suite
	dbMixin
}

func (s *MixSuite) TestOwn() { s.Require().Equal(1, s.n) }

func TestMix(t *testing.T) { suite.Run(t, new(MixSuite)) }
`

func (s *MigrateCompileTestSuite) TestPromotedMethods(t *gotest.T) {
	run := s.migrate(t, map[string]string{"p/a_test.go": mixinSrc})

	t.It("marks the embedded type whose hook and test gotest would not call", func(it *gotest.T) {
		gotest.Contains(it, run.files["p/a_test.go"], "\t// TODO(gotest-migrate): SetupTest, TestShared are promoted from dbMixin; gotest calls only methods MixTestSuite declares itself, so they no longer run — declare them on MixTestSuite (hooks under their gotest names, with t) and call the embedded ones\n\tdbMixin\n")
		gotest.Equal(it, 1, run.markers())
	})
}

const baseSuiteSrc = `package testutil

import "github.com/stretchr/testify/suite"

type BaseSuite struct{ suite.Suite }

func (b *BaseSuite) SetupSuite() {}
`

const usesBaseSrc = `package api

import (
	"testing"

	"example.com/m/testutil"
	"github.com/stretchr/testify/suite"
)

type APISuite struct{ testutil.BaseSuite }

func (s *APISuite) TestA() { s.Require().True(true) }

func TestAPI(t *testing.T) { suite.Run(t, new(APISuite)) }
`

func (s *MigrateCompileTestSuite) TestForeignBaseSuite(t *gotest.T) {
	run := s.migrate(t, map[string]string{
		"testutil/base.go": baseSuiteSrc,
		"api/a_test.go":    usesBaseSrc,
	})

	t.It("refuses the suite with a marker, never as nothing found", func(it *gotest.T) {
		gotest.Len(it, run.results, 1)
		gotest.Contains(it, run.results[0].Refusal, "APISuite embeds testutil.BaseSuite, a testify suite of another package")
		gotest.Equal(it, 1, run.markers())
		gotest.Contains(it, run.files["api/a_test.go"], "// TODO(gotest-migrate): APISuite embeds testutil.BaseSuite")
	})

	t.It("leaves the code compiling as it was", func(it *gotest.T) {
		gotest.Empty(it, compiles(it, run.dir))
	})
}

const mappedSrc = `package p

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type MapSuite struct{ suite.Suite }

func (s *MapSuite) TestAll() {
	x := 1
	p, q := &x, &x
	s.NotZero(x)
	s.Same(p, q)
	s.NotSame(p, new(int))
	s.ElementsMatch([]int{1, 2}, []int{2, 1})
	s.Subset([]int{1, 2}, []int{1})
	s.NotContains("abc", "z")
	s.Regexp("^a", "abc")
	s.InDelta(1.0, 1.05, 0.1)
	s.JSONEq(` + "`" + `{"a":1}` + "`" + `, ` + "`" + `{ "a": 1 }` + "`" + `)
	s.Panics(func() { panic("x") })
	s.Equal(1, x)
	assert.NotZero(s.T(), x)
}

func (s *MapSuite) TestSub() {
	s.Run("inner", func() {
		s.Require().Equal(2, 2)
		s.T().Log("in inner")
	})
}

func TestMap(t *testing.T) { suite.Run(t, new(MapSuite)) }
`

func (s *MigrateCompileTestSuite) TestMappedAssertions(t *gotest.T) {
	run := s.migrate(t, map[string]string{"p/a_test.go": mappedSrc})
	out := run.files["p/a_test.go"]

	t.It("maps every assertion gotest has, the suite's own methods included", func(it *gotest.T) {
		for _, line := range []string{
			"gotest.NotZero(t, x)", "gotest.Same(t, p, q)", "gotest.NotSame(t, p, new(int))",
			"gotest.ElementsMatch(t, []int{1, 2}, []int{2, 1})", "gotest.Subset(t, []int{1, 2}, []int{1})",
			`gotest.NotContains(t, "abc", "z")`, `gotest.Regexp(t, "^a", "abc")`, "gotest.InDelta(t, 1.0, 1.05, 0.1)",
			"gotest.JSONEq(t, `{\"a\":1}`, `{ \"a\": 1 }`)", `gotest.Panics(t, func() { panic("x") })`, "gotest.Equal(t, 1, x)",
		} {
			gotest.Contains(it, out, "\t"+line+"\n")
		}
		gotest.NotContains(it, out, "testify")
	})

	t.It("turns s.Run into t.It, whose own t the assertions inside name", func(it *gotest.T) {
		gotest.Contains(it, out, "\tt.It(\"inner\", func(t *gotest.T) {\n\t\tgotest.Equal(t, 2, 2)\n\t\tt.T().Log(\"in inner\")\n\t})\n")
	})

	t.It("compiles with no marker and passes", func(it *gotest.T) {
		gotest.Zero(it, run.markers())
		stdout, stderr, code := s.runSuites(it, run.dir, "./p")
		gotest.Equal(it, 0, code, stdout+stderr)
		gotest.Contains(it, stdout, "--- PASS: TestMapTestSuite/TestSub/inner")
	})
}

const taggedHelperSrc = `//go:build integration

package p

func (s *TagSuite) helper() { s.SetupTest() }
`

const taggedSuiteSrc = `package p

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type TagSuite struct{ suite.Suite }

func (s *TagSuite) SetupTest() {}
func (s *TagSuite) TestA()     {}

func TestTag(t *testing.T) { suite.Run(t, new(TagSuite)) }
`

func (s *MigrateCompileTestSuite) TestBuildTags(t *gotest.T) {
	run := s.migrate(t, map[string]string{
		"p/a_test.go":            taggedSuiteSrc,
		"p/helper_integ_test.go": taggedHelperSrc,
	})

	t.It("type-checks a tagged file in its own build and marks its error", func(it *gotest.T) {
		gotest.Contains(it, run.files["p/helper_integ_test.go"], "// TODO(gotest-migrate): does not compile: not enough arguments in call to s.BeforeEach")
		gotest.Empty(it, unmarkedErrors(it, run.dir, "-tags=integration"))
	})

	t.It("leaves the default build compiling", func(it *gotest.T) {
		gotest.Empty(it, compiles(it, run.dir))
	})
}

func (s *MigrateCompileTestSuite) TestUncompiledPackage(t *gotest.T) {
	src := strings.Replace(taggedSuiteSrc, "func (s *TagSuite) TestA()     {}", "func (s *TagSuite) TestA()     { undefined() }", 1)
	run := s.migrate(t, map[string]string{"p/a_test.go": src})

	t.It("reports the package as not compiling, with its error", func(it *gotest.T) {
		gotest.Len(it, run.results, 1)
		gotest.Len(it, run.results[0].Uncompiled, 1)
		gotest.Contains(it, run.results[0].Uncompiled[0], "undefined: undefined")
	})

	t.It("leaves it untouched", func(it *gotest.T) {
		gotest.Equal(it, src, run.files["p/a_test.go"])
	})
}

const stdOnlySrc = `package p

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
)

type StdSuite struct{ suite.Suite }

func (s *StdSuite) TestA() { fmt.Println("x"); s.Require().True(true) }

func TestStd(t *testing.T) { suite.Run(t, new(StdSuite)) }
`

func (s *MigrateCompileTestSuite) TestImportGrouping(t *gotest.T) {
	run := s.migrate(t, map[string]string{"p/a_test.go": stdOnlySrc})

	t.It("puts the gotest import apart from the standard library", func(it *gotest.T) {
		gotest.Contains(it, run.files["p/a_test.go"], "import (\n\t\"fmt\"\n\n\t\"github.com/mvrahden/go-test/pkg/gotest\"\n)\n")
	})
}

func (s *MigrateCompileTestSuite) TestTagOnlyDirectory(t *gotest.T) {
	src := "//go:build integration\n\n" + stdOnlySrc
	for _, pattern := range []string{"./tagonly", "./tagonly/...", "./...", "example.com/m/tagonly", "example.com/m/..."} {
		t.When("the pattern is "+pattern, func(w *gotest.T) {
			root := w.TempDir()
			gotest.NoError(w, s.Mod.Write(root, map[string]string{"tagonly/a_test.go": src}))
			testkit.ScrubActionsEnv()
			cmd := exec.CommandContext(w.Context(), s.CLI.Binary, "migrate", "--dry-run", pattern) //nolint:gosec // G204: the CLI under test
			cmd.Dir = root
			cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=", "PWD="+root)
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			_ = cmd.Run()

			w.It("migrates the directory, under every spelling", func(it *gotest.T) {
				gotest.Equal(it, 0, cmd.ProcessState.ExitCode(), stdout.String()+stderr.String())
				gotest.Contains(it, stdout.String(), "Would migrate 1 suites:\n  "+filepath.Join("tagonly", "a_test.go")+": StdSuite → StdTestSuite\n")
				gotest.Empty(it, stderr.String())
			})
		})
	}
}

// otherOSSrc builds only on an OS no CI host runs, so no configuration
// migrate checks compiles it.
const otherOSSrc = `//go:build plan9

package p

func (s *TagSuite) onPlan9() { s.Require().True(true) }
`

func (s *MigrateCompileTestSuite) TestUncheckedFile(t *gotest.T) {
	run := s.migrate(t, map[string]string{
		"p/a_test.go":     taggedSuiteSrc,
		"p/plan9_test.go": otherOSSrc,
	})

	t.It("marks the file it cannot type-check", func(it *gotest.T) {
		gotest.Contains(it, run.files["p/plan9_test.go"], "// TODO(gotest-migrate): migrate cannot type-check this file: //go:build plan9 keeps it out of every build it checked — migrate it by hand\npackage p\n")
	})

	t.It("leaves the suite it refers to as it is", func(it *gotest.T) {
		gotest.Contains(it, run.files["p/a_test.go"], "type TagSuite struct{ suite.Suite }")
		gotest.Empty(it, compiles(it, run.dir))
	})
}

const unrunSrc = `package p

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type RunSuite struct{ suite.Suite }

func (s *RunSuite) TestA() {}

type IdleSuite struct{ suite.Suite }

func (s *IdleSuite) TestB() {}

func TestRun(t *testing.T) { suite.Run(t, new(RunSuite)) }
`

func (s *MigrateCompileTestSuite) TestSuiteWithoutRunner(t *gotest.T) {
	run := s.migrate(t, map[string]string{"p/a_test.go": unrunSrc})

	t.It("marks the suite testify never ran, which gotest would", func(it *gotest.T) {
		gotest.Contains(it, run.files["p/a_test.go"], "// TODO(gotest-migrate): no suite.Run ran IdleSuite, so testify never did; gotest runs every suite — prefix it with X_ to keep it off\ntype IdleTestSuite struct{}")
		gotest.Equal(it, 1, run.markers())
	})
}

const cycleXSrc = `package p_test

import (
	"testing"

	"example.com/m/p"
	"example.com/m/p/util"
	"github.com/stretchr/testify/suite"
)

type CycSuite struct{ suite.Suite }

func (s *CycSuite) TestA() {
	var got p.Thing = util.Make()
	s.Require().Equal(p.Thing{N: 1}, got)
}

func TestCyc(t *testing.T) { suite.Run(t, new(CycSuite)) }
`

func (s *MigrateCompileTestSuite) TestRecompiledDependency(t *gotest.T) {
	run := s.migrate(t, map[string]string{
		"p/p.go":         "package p\n\ntype Thing struct{ N int }\n",
		"p/in_test.go":   "package p\n\nfunc helper() int { return 1 }\n",
		"p/util/util.go": "package util\n\nimport \"example.com/m/p\"\n\nfunc Make() p.Thing { return p.Thing{N: 1} }\n",
		"p/x_test.go":    cycleXSrc,
	}, "p")

	t.It("type-checks a test whose dependency imports the package back", func(it *gotest.T) {
		gotest.Len(it, run.results, 1)
		gotest.Equal(it, "CycTestSuite", run.results[0].NewName)
		gotest.Zero(it, run.markers())
		gotest.Empty(it, compiles(it, run.dir))
	})
}

const unexportedSrc = `package p

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
)

type thingSuite struct{ suite.Suite }

func (s *thingSuite) TestA() { fmt.Println("RAN thing TestA") }

func TestThing(t *testing.T) { suite.Run(t, new(thingSuite)) }
`

func (s *MigrateCompileTestSuite) TestUnexportedSuite(t *gotest.T) {
	t.When("the exported name is free", func(w *gotest.T) {
		run := s.migrate(w, map[string]string{"p/a_test.go": unexportedSrc})

		w.It("exports the suite and compiles with no marker", func(it *gotest.T) {
			gotest.Len(it, run.results, 1)
			gotest.Equal(it, "ThingTestSuite", run.results[0].NewName)
			gotest.Zero(it, run.markers())
			gotest.Empty(it, compiles(it, run.dir))
		})

		w.It("runs its tests", func(it *gotest.T) {
			stdout, stderr, code := s.runSuites(it, run.dir, "./p")
			gotest.Equal(it, 0, code, stdout+stderr)
			gotest.Contains(it, stdout, "RAN thing TestA")
		})
	})

	t.When("the exported name is taken", func(w *gotest.T) {
		src := unexportedSrc + "\ntype ThingTestSuite struct{}\n"
		run := s.migrate(w, map[string]string{"p/a_test.go": src})

		w.It("refuses the suite with a marker", func(it *gotest.T) {
			gotest.Contains(it, run.files["p/a_test.go"], "// TODO(gotest-migrate): renaming thingSuite to ThingTestSuite collides with another declaration")
			gotest.Equal(it, 1, run.markers())
		})
	})
}

func (s *MigrateCompileTestSuite) TestNameTakenThroughDotImport(t *gotest.T) {
	x := "package p_test\n\nimport . \"example.com/m/p\"\n\ntype ThingTestSuite struct{}\n\nvar _ = Exported\n"
	run := s.migrate(t, map[string]string{
		"p/p.go":      "package p\n\nconst Exported = 1\n",
		"p/a_test.go": unexportedSrc,
		"p/x_test.go": x,
	})

	t.It("refuses the suite instead of breaking the external package", func(it *gotest.T) {
		gotest.Contains(it, run.files["p/a_test.go"], "// TODO(gotest-migrate): renaming thingSuite to ThingTestSuite collides with another declaration")
		gotest.Equal(it, x, run.files["p/x_test.go"])
		gotest.Equal(it, 1, run.markers())
	})
}

func (s *MigrateCompileTestSuite) TestWriteGuard(t *gotest.T) {
	var before, inside string
	var calls int
	var path string
	opts := migrate.Options{WriteGuard: func(write func() error) error {
		calls++
		before = string(gotest.Must(os.ReadFile(path)))
		err := write()
		inside = string(gotest.Must(os.ReadFile(path)))
		return err
	}}
	root := t.TempDir()
	path = filepath.Join(root, "p", "a_test.go")
	gotest.NoError(t, s.Mod.Write(root, map[string]string{"p/a_test.go": unexportedSrc}))
	_, err := migrate.MigratePackages([]string{filepath.Join(root, "p")}, opts)

	t.It("writes the files inside the guard, once", func(it *gotest.T) {
		gotest.NoError(it, err)
		gotest.Equal(it, 1, calls)
		gotest.Equal(it, unexportedSrc, before)
		gotest.Contains(it, inside, "type ThingTestSuite struct")
	})
}

func (s *MigrateCompileTestSuite) TestNameGotestWouldNotRun(t *gotest.T) {
	for t, tc := range gotest.Each(t, []struct {
		Desc, name, becomes string
	}{
		{"a suite named testSuite", "testSuite", "TestSuite"},
		{"a suite whose name reads as focused", "f_barSuite", "F_barTestSuite"},
		{"a suite whose name reads as excluded", "x_bazSuite", "X_bazTestSuite"},
	}) {
		src := strings.ReplaceAll(strings.ReplaceAll(unexportedSrc, "thingSuite", tc.name), "TestThing(", "TestIt(")
		run := s.migrate(t, map[string]string{"p/a_test.go": src})
		gotest.Contains(t, run.files["p/a_test.go"], "// TODO(gotest-migrate): "+tc.name+" would become "+tc.becomes+", which gotest does not run as a plain suite")
		gotest.Equal(t, 1, run.markers())
	}
}

const nameClashSrc = `package p

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type FooSuite struct{ suite.Suite }

func (s *FooSuite) TestB() {}

type FooTestSuite struct{ suite.Suite }

func (s *FooTestSuite) TestA() {}

func TestFoo(t *testing.T)  { suite.Run(t, new(FooTestSuite)) }
func TestFoo2(t *testing.T) { suite.Run(t, new(FooSuite)) }
`

func (s *MigrateCompileTestSuite) TestRenameOntoSuite(t *gotest.T) {
	run := s.migrate(t, map[string]string{"p/a_test.go": nameClashSrc})
	out := run.files["p/a_test.go"]

	t.It("blames the suite being renamed, not the one holding the name", func(it *gotest.T) {
		gotest.Contains(it, out, "// TODO(gotest-migrate): renaming FooSuite to FooTestSuite collides with another declaration")
		gotest.NotContains(it, out, "renaming FooTestSuite")
		gotest.Equal(it, 1, run.markers())
	})
}

const ptrEmbedSrc = `package p

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
)

type PtrSuite struct {
	*suite.Suite
	n int
}

func (s *PtrSuite) SetupTest() { s.n = 1 }

func (s *PtrSuite) TestA() {
	s.Require().Equal(1, s.n)
	fmt.Println("RAN ptr TestA")
}

func TestPtr(t *testing.T) { suite.Run(t, &PtrSuite{Suite: new(suite.Suite)}) }
`

func (s *MigrateCompileTestSuite) TestPointerEmbed(t *gotest.T) {
	run := s.migrate(t, map[string]string{"p/a_test.go": ptrEmbedSrc})
	out := run.files["p/a_test.go"]

	t.It("converts it as testify's suite and drops the runner that sets it", func(it *gotest.T) {
		gotest.Contains(it, out, "type PtrTestSuite struct {\n\tn int\n}")
		gotest.NotContains(it, out, "suite.Run")
		gotest.Zero(it, run.markers())
		gotest.Empty(it, compiles(it, run.dir))
	})

	t.It("runs the test with its hook", func(it *gotest.T) {
		stdout, stderr, code := s.runSuites(it, run.dir, "./p")
		gotest.Equal(it, 0, code, stdout+stderr)
		gotest.Contains(it, stdout, "RAN ptr TestA")
	})
}

const reservedSrc = `package p

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type ResSuite struct {
	suite.Suite
	n int
}

func (s *ResSuite) SetupTest() { s.n = 1 }

func (s *ResSuite) BenchmarkInput() int { return s.n }

func (s *ResSuite) FuzzSeed(n int) int { return n }

func (s *ResSuite) SuiteGuard() string { return "skip" }

func (s *ResSuite) AfterAll() {}

func (s *ResSuite) F_TestHelper() {}

func (s *ResSuite) Test() {}

func (s *ResSuite) TestA() {
	s.Require().Equal(1, s.BenchmarkInput()+s.FuzzSeed(0))
	s.Require().Equal("skip", s.SuiteGuard())
}

func TestRes(t *testing.T) { suite.Run(t, new(ResSuite)) }
`

func (s *MigrateCompileTestSuite) TestReservedNames(t *gotest.T) {
	run := s.migrate(t, map[string]string{"p/a_test.go": reservedSrc})
	out := run.files["p/a_test.go"]

	t.It("converts testify's own hook without a marker", func(it *gotest.T) {
		gotest.Contains(it, out, "\nfunc (s *ResTestSuite) BeforeEach(t *gotest.T) { s.n = 1 }\n")
	})

	t.It("marks each helper named as gotest reserves", func(it *gotest.T) {
		for _, want := range []string{
			"// TODO(gotest-migrate): gotest takes a suite method named BenchmarkInput for a benchmark; testify did not — rename it\nfunc (s *ResTestSuite) BenchmarkInput() int",
			"// TODO(gotest-migrate): gotest takes a suite method named FuzzSeed for a fuzz target; testify did not — rename it\nfunc (s *ResTestSuite) FuzzSeed(n int) int",
			"// TODO(gotest-migrate): gotest takes a suite method named SuiteGuard for the suite's guard; testify did not — rename it\nfunc (s *ResTestSuite) SuiteGuard() string",
			"// TODO(gotest-migrate): gotest takes a suite method named AfterAll for a lifecycle hook; testify did not — rename it\nfunc (s *ResTestSuite) AfterAll() {}",
			"// TODO(gotest-migrate): gotest takes a suite method named F_TestHelper for a focused test; testify did not — rename it\nfunc (s *ResTestSuite) F_TestHelper() {}",
			"// TODO(gotest-migrate): testify ran Test as a test; gotest runs only methods named Test followed by more, so it no longer runs — rename it, e.g. TestAll\nfunc (s *ResTestSuite) Test(t *gotest.T) {}",
		} {
			gotest.Contains(it, out, want)
		}
		gotest.Equal(it, 6, run.markers())
	})
}

func (s *MigrateCompileTestSuite) TestReservedNameAHookBecomes(t *gotest.T) {
	src := "package p\n\nimport (\n\t\"testing\"\n\n\t\"github.com/stretchr/testify/suite\"\n)\n\n" +
		"type HkSuite struct {\n\tsuite.Suite\n\tn int\n}\n\n" +
		"func (s *HkSuite) SetupTest() { s.n = 1 }\n\nfunc (s *HkSuite) BeforeEach() { s.n = 2 }\n\n" +
		"func (s *HkSuite) TestA() {}\n\nfunc TestHkSuite(t *testing.T) { suite.Run(t, new(HkSuite)) }\n"
	out := s.migrate(t, map[string]string{"p/a_test.go": src}).files["p/a_test.go"]

	t.It("tells to rename the helper, not the hook", func(it *gotest.T) {
		gotest.Contains(it, out, "// TODO(gotest-migrate): testify's SetupTest becomes BeforeEach, which the suite already declares — rename the other BeforeEach")
		gotest.NotContains(it, out, "testify did not — rename it")
	})
}

const formattedSrc = `package p

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type FmtSuite struct{ suite.Suite }

func (s *FmtSuite) TestPass() {
	var err error
	msgs := []any{"ctx %d", 1}
	s.Require().Equalf(2, 1+1, "sum of %d and %d", 1, 1)
	s.Require().NoErrorf(err, "load %s", "x")
	s.Truef(true, "plain")
	assert.Lenf(s.T(), []int{1}, 1, "len %v", 1)
	require.Emptyf(s.T(), "", "empty")
	s.Require().Equal(1, 1, msgs...)
	s.Require().Equalf(1, 1, "spread %d", msgs[1:]...)
}

func (s *FmtSuite) TestFail() {
	s.Require().Equalf(1, 2, "want %d got %d", 1, 2)
}

func TestFmt(t *testing.T) { suite.Run(t, new(FmtSuite)) }
`

func (s *MigrateCompileTestSuite) TestFormattedAssertions(t *gotest.T) {
	run := s.migrate(t, map[string]string{"p/a_test.go": formattedSrc})
	out := run.files["p/a_test.go"]

	t.It("maps each ...f variant to its assertion, the message and args trailing", func(it *gotest.T) {
		gotest.Contains(it, out, "\tgotest.Equal(t, 2, 1+1, \"sum of %d and %d\", 1, 1)\n")
		gotest.Contains(it, out, "\tgotest.NoError(t, err, \"load %s\", \"x\")\n")
		gotest.Contains(it, out, "\tgotest.True(t, true, \"plain\")\n")
		gotest.Contains(it, out, "\tgotest.Len(t, []int{1}, 1, \"len %v\", 1)\n")
		gotest.Contains(it, out, "\tgotest.Empty(t, \"\", \"empty\")\n")
	})

	t.It("keeps a spread message spread", func(it *gotest.T) {
		gotest.Contains(it, out, "\tgotest.Equal(t, 1, 1, msgs...)\n")
	})

	t.It("marks a spread after a format, which has no gotest spelling", func(it *gotest.T) {
		gotest.Contains(it, out, "\t// TODO(gotest-migrate): Equalf spreads args after its message, which gotest.Equal cannot take — rewrite as gotest.Equal(t, ..., append([]any{msg}, args...)...)\n\ts.Require().Equalf(1, 1, \"spread %d\", msgs[1:]...)\n")
		gotest.Equal(it, 1, run.markers())
	})

	t.It("reports a failure with the message formatted as testify did", func(it *gotest.T) {
		path := filepath.Join(run.dir, "p", "a_test.go")
		fixed := regexp.MustCompile(`\t// TODO.*\n\ts\.Require\(\)\.Equalf.*\n`).ReplaceAllString(out, "")
		gotest.NoError(it, os.WriteFile(path, []byte(fixed), 0o600))
		stdout, stderr, code := s.runSuites(it, run.dir, "./p")
		gotest.Equal(it, 1, code, stdout+stderr)
		gotest.Contains(it, stdout, "--- PASS: TestFmtTestSuite/TestPass")
		gotest.Contains(it, stdout, "message: want 1 got 2\n")
	})
}
