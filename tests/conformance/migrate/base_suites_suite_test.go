package migrate_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/gotestcli"
)

// MigrateBaseSuitesTestSuite: a testify suite built on a base suite migrates
// with its base, which becomes a helper the child calls.
type MigrateBaseSuitesTestSuite struct {
	Mod *gotestcli.TestifyModuleSharedFixture
}

func (s *MigrateBaseSuitesTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

// baseSrc is a base no runner runs, named as gotest names a suite.
const baseSrc = `package p

import (
	"fmt"

	"github.com/stretchr/testify/suite"
)

type BaseTestSuite struct {
	suite.Suite
	rows []string
}

func (b *BaseTestSuite) SetupTest() {
	b.rows = []string{"row"}
	fmt.Println("RAN base SetupTest")
}

func (b *BaseTestSuite) TestShared() {
	b.Len(b.rows, 1)
	fmt.Println("RAN base TestShared")
}

func (b *BaseTestSuite) count() int {
	b.Require().NotEmpty(b.rows)
	return len(b.rows)
}
`

const childSrc = `package p

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
)

type ChildSuite struct {
	BaseTestSuite
}

func (s *ChildSuite) TestOwn() {
	s.Equal(1, s.count())
	fmt.Println("RAN child TestOwn")
}

func TestChild(t *testing.T) { suite.Run(t, new(ChildSuite)) }
`

const outsideBaseSrc = `package testutil

import "github.com/stretchr/testify/suite"

type BaseSuite struct{ suite.Suite }

func (b *BaseSuite) SetupSuite() {}
`

const outsideChildSrc = `package api

import (
	"testing"

	"example.com/m/testutil"
	"github.com/stretchr/testify/suite"
)

type APISuite struct{ testutil.BaseSuite }

func (s *APISuite) TestA() { s.Require().True(true) }

func TestAPI(t *testing.T) { suite.Run(t, new(APISuite)) }
`

var moduleEnv = []string{"GOWORK=off", "GOFLAGS="}

func (s *MigrateBaseSuitesTestSuite) TestChildOfABase(t *gotest.T) {
	t.When("a testify suite embeds a base suite of the migrated test files", func(w *gotest.T) {
		dir := w.TempDir()
		gotest.NoError(w, s.Mod.Write(dir, map[string]string{"p/base_test.go": baseSrc, "p/child_test.go": childSrc}))
		mig, err := testkit.RunCLIWithEnv(w.Context(), dir, moduleEnv, "migrate", "./...")
		gotest.NoError(w, err)
		base := readFile(w, dir, "p/base_test.go")
		child := readFile(w, dir, "p/child_test.go")
		run, err := testkit.RunCLIWithEnv(w.Context(), dir, moduleEnv, "-json", "./...")
		gotest.NoError(w, err)

		w.It("the migrated child compiles and gotest runs the base's hooks and tests on it", func(it *gotest.T) {
			gotest.Contains(it, child, "type ChildTestSuite struct {\n\tbase BaseTestSuiteHelpers\n}")
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Equal(it, "pass", run.Verdict("TestChildTestSuite/TestShared"), "stdout: %s", run.Stdout)
			gotest.Equal(it, "pass", run.Verdict("TestChildTestSuite/TestOwn"), "stdout: %s", run.Stdout)
			gotest.Contains(it, run.Stdout, "RAN base SetupTest")
		})

		w.It("the base becomes a helper type gotest does not run as a suite", func(it *gotest.T) {
			gotest.Contains(it, base, "type BaseTestSuiteHelpers struct {\n\trows []string\n}")
			gotest.Contains(it, base, "func (b *BaseTestSuiteHelpers) count(t *gotest.T) int {\n\tgotest.NotEmpty(t, b.rows)")
			gotest.Empty(it, run.Verdict("TestBaseTestSuite"), "stdout: %s", run.Stdout)
			gotest.Empty(it, run.Verdict("TestBaseTestSuiteHelpers"), "stdout: %s", run.Stdout)
		})

		w.It("migrate leaves no marker for the embedding", func(it *gotest.T) {
			gotest.Equal(it, 0, mig.Exit, "stdout: %s\nstderr: %s", mig.Stdout, mig.Stderr)
			gotest.NotContains(it, base+child, "TODO(gotest-migrate)")
		})

		w.It("gotest lint reports no testify finding on the migrated code", func(it *gotest.T) {
			lint, err := testkit.RunCLIWithEnv(it.Context(), dir, moduleEnv, "lint", "-json", "./...")
			gotest.NoError(it, err)
			gotest.Empty(it, testifyFindings(it, lint.Stdout))
		})
	})
}

func (s *MigrateBaseSuitesTestSuite) TestBaseOutsideTheMigratedDirectory(t *gotest.T) {
	t.When("a testify suite embeds a base suite of a package in another directory", func(w *gotest.T) {
		w.It("migrate exits 2 naming the base's package and writes nothing", func(it *gotest.T) {
			dir := it.TempDir()
			files := map[string]string{"testutil/base.go": outsideBaseSrc, "api/a_test.go": outsideChildSrc}
			gotest.NoError(it, s.Mod.Write(dir, files))
			mig, err := testkit.RunCLIWithEnv(it.Context(), dir, moduleEnv, "migrate", "./...")
			gotest.NoError(it, err)
			gotest.Equal(it, 2, mig.Exit, "stdout: %s\nstderr: %s", mig.Stdout, mig.Stderr)
			gotest.Contains(it, mig.Stderr, "migrate: "+filepath.Join("api", "a_test.go")+":10: APISuite embeds testutil.BaseSuite, a testify suite declared outside the test files being migrated, in package example.com/m/testutil")
			for name, src := range files {
				gotest.Equal(it, src, readFile(it, dir, name))
			}
		})
	})
}

const pointerBaseSrc = `package p

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
)

type BaseSuite struct {
	suite.Suite
	n int
}

func (b *BaseSuite) SetupTest() { b.n = 1 }

func (b *BaseSuite) TestShared() {
	b.Equal(1, b.n)
	fmt.Println("RAN base TestShared")
}

type ChildSuite struct {
	*BaseSuite
}

func (s *ChildSuite) TestOwn() {
	s.Equal(1, s.n)
	keep(s.BaseSuite)
}

func keep(b *BaseSuite) { fmt.Println("RAN child TestOwn", b.n) }

func TestChild(t *testing.T) { suite.Run(t, &ChildSuite{BaseSuite: &BaseSuite{}}) }
`

const genericBaseSrc = `package p

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
)

type BaseSuite[T any] struct {
	suite.Suite
	rows []T
}

func (b *BaseSuite[T]) SetupTest() {
	var zero T
	b.rows = []T{zero}
}

func (b *BaseSuite[T]) TestShared() {
	b.Len(b.rows, 1)
	fmt.Println("RAN base TestShared")
}

type IntSuite struct {
	BaseSuite[int]
}

func (s *IntSuite) TestOwn() {
	s.Equal(0, s.rows[0])
	fmt.Println("RAN child TestOwn")
}

func TestInt(t *testing.T) { suite.Run(t, new(IntSuite)) }
`

const childMethodSrc = `package p

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
)

type BaseSuite struct{ suite.Suite }

func (b *BaseSuite) check(v int) { b.Require().Equal(3, v) }

type ChildSuite struct {
	BaseSuite
}

func (s *ChildSuite) checkSum(a, b int) { s.check(a + b) }

func (s *ChildSuite) TestOwn() {
	s.checkSum(1, 2)
	fmt.Println("RAN child TestOwn")
}

func TestChild(t *testing.T) { suite.Run(t, new(ChildSuite)) }
`

const localEmbedSrc = `package p

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type BaseSuite struct{ suite.Suite }

func (b *BaseSuite) TestShared() { b.True(true) }

func TestLocal(t *testing.T) {
	type localSuite struct{ BaseSuite }
	suite.Run(t, new(localSuite))
}

func TestAnonymous(t *testing.T) { suite.Run(t, &struct{ BaseSuite }{}) }
`

const nonTestBaseSrc = `package p

import "github.com/stretchr/testify/suite"

type BaseSuite struct{ suite.Suite }

func (b *BaseSuite) SetupSuite() {}
`

const nonTestChildSrc = `package p

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type APISuite struct{ BaseSuite }

func (s *APISuite) TestA() { s.Require().True(true) }

func TestAPI(t *testing.T) { suite.Run(t, new(APISuite)) }
`

// migrateAndRun migrates files in a fresh module, then runs it with -json.
func (s *MigrateBaseSuitesTestSuite) migrateAndRun(t *gotest.T, files map[string]string) (dir string, mig, run testkit.Run) {
	dir = t.TempDir()
	gotest.NoError(t, s.Mod.Write(dir, files))
	mig, err := testkit.RunCLIWithEnv(t.Context(), dir, moduleEnv, "migrate", "./...")
	gotest.NoError(t, err)
	run, err = testkit.RunCLIWithEnv(t.Context(), dir, moduleEnv, "-json", "./...")
	gotest.NoError(t, err)
	return dir, mig, run
}

func (s *MigrateBaseSuitesTestSuite) TestABaseEmbeddedByPointer(t *gotest.T) {
	t.When("a testify suite embeds a base suite of the migrated test files by a pointer its suite.Run sets to a fresh base", func(w *gotest.T) {
		w.It("the migrated child holds the base by value and gotest runs the base's hooks and tests on it", func(it *gotest.T) {
			dir, mig, run := s.migrateAndRun(it, map[string]string{"p/a_test.go": pointerBaseSrc})
			out := readFile(it, dir, "p/a_test.go")
			gotest.Equal(it, 0, mig.Exit, "stdout: %s\nstderr: %s\n%s", mig.Stdout, mig.Stderr, out)
			gotest.Contains(it, out, "type ChildTestSuite struct {\n\tbase BaseSuite\n}")
			gotest.Contains(it, out, "keep(&s.base)")
			gotest.NotContains(it, out, "TODO(gotest-migrate)")
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Equal(it, "pass", run.Verdict("TestChildTestSuite/TestShared"), "stdout: %s", run.Stdout)
			gotest.Equal(it, "pass", run.Verdict("TestChildTestSuite/TestOwn"), "stdout: %s", run.Stdout)
			gotest.Contains(it, run.Stdout, "RAN child TestOwn 1")
		})
	})
}

func (s *MigrateBaseSuitesTestSuite) TestAGenericBase(t *gotest.T) {
	t.When("a testify suite embeds an instance of a generic base suite", func(w *gotest.T) {
		w.It("the base stays a generic helper type and gotest runs its hooks and tests on the child", func(it *gotest.T) {
			dir, mig, run := s.migrateAndRun(it, map[string]string{"p/a_test.go": genericBaseSrc})
			out := readFile(it, dir, "p/a_test.go")
			gotest.Equal(it, 0, mig.Exit, "stdout: %s\nstderr: %s\n%s", mig.Stdout, mig.Stderr, out)
			gotest.Contains(it, out, "type BaseSuite[T any] struct {\n\trows []T\n}")
			gotest.Contains(it, out, "type IntTestSuite struct {\n\tbase BaseSuite[int]\n}")
			gotest.NotContains(it, out, "TODO(gotest-migrate)")
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Equal(it, "pass", run.Verdict("TestIntTestSuite/TestShared"), "stdout: %s", run.Stdout)
			gotest.Equal(it, "pass", run.Verdict("TestIntTestSuite/TestOwn"), "stdout: %s", run.Stdout)
		})
	})
}

func (s *MigrateBaseSuitesTestSuite) TestAChildsMethodCallingTheBase(t *gotest.T) {
	t.When("a method of the child without t calls a base method that gains t", func(w *gotest.T) {
		w.It("the method gains t, its callers pass it, and the migrated child runs without a marker", func(it *gotest.T) {
			dir, mig, run := s.migrateAndRun(it, map[string]string{"p/a_test.go": childMethodSrc})
			out := readFile(it, dir, "p/a_test.go")
			gotest.Equal(it, 0, mig.Exit, "stdout: %s\nstderr: %s\n%s", mig.Stdout, mig.Stderr, out)
			gotest.Contains(it, out, "func (s *ChildTestSuite) checkSum(t *gotest.T, a, b int) { s.base.check(t, a+b) }")
			gotest.Contains(it, out, "\ts.checkSum(t, 1, 2)\n")
			gotest.NotContains(it, out, "TODO(gotest-migrate)")
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Equal(it, "pass", run.Verdict("TestChildTestSuite/TestOwn"), "stdout: %s", run.Stdout)
		})
	})
}

func (s *MigrateBaseSuitesTestSuite) TestAnEmbeddingInALocalOrAnonymousStruct(t *gotest.T) {
	t.When("a struct declared inside a function, or an anonymous struct, embeds a base suite", func(w *gotest.T) {
		w.It("migrate leaves its file unconverted under a marker saying gotest runs only suites declared at package level", func(it *gotest.T) {
			dir := it.TempDir()
			gotest.NoError(it, s.Mod.Write(dir, map[string]string{"p/a_test.go": localEmbedSrc}))
			mig, err := testkit.RunCLIWithEnv(it.Context(), dir, moduleEnv, "migrate", "./...")
			gotest.NoError(it, err)
			out := readFile(it, dir, "p/a_test.go")
			gotest.Equal(it, 1, mig.Exit, "stdout: %s\nstderr: %s", mig.Stdout, mig.Stderr)
			gotest.Regexp(it, `\t// TODO\(gotest-migrate\): localSuite embeds BaseSuite inside a function; gotest runs only suites declared at package level[^\n]*\n\ttype localSuite`, out)
			gotest.Regexp(it, `// TODO\(gotest-migrate\): an anonymous struct embeds BaseSuite; gotest runs only suites declared at package level[^\n]*\nfunc TestAnonymous`, out)
			gotest.Equal(it, localEmbedSrc, stripMarkers(out))
		})
	})
}

func (s *MigrateBaseSuitesTestSuite) TestABaseInANonTestFile(t *gotest.T) {
	t.When("a testify suite embeds a base suite declared in a non-test file of its directory", func(w *gotest.T) {
		w.It("migrate exits 1 and leaves the child's file unconverted under a marker naming that file", func(it *gotest.T) {
			dir := it.TempDir()
			gotest.NoError(it, s.Mod.Write(dir, map[string]string{"p/base.go": nonTestBaseSrc, "p/a_test.go": nonTestChildSrc}))
			mig, err := testkit.RunCLIWithEnv(it.Context(), dir, moduleEnv, "migrate", "./...")
			gotest.NoError(it, err)
			out := readFile(it, dir, "p/a_test.go")
			gotest.Equal(it, 1, mig.Exit, "stdout: %s\nstderr: %s", mig.Stdout, mig.Stderr)
			gotest.Regexp(it, `// TODO\(gotest-migrate\): APISuite embeds BaseSuite, declared in base\.go, which is not a test file;[^\n]*\ntype APISuite struct`, out)
			gotest.Equal(it, nonTestChildSrc, stripMarkers(out))
			gotest.Equal(it, nonTestBaseSrc, readFile(it, dir, "p/base.go"))
		})
	})
}

// stripMarkers drops the TODO(gotest-migrate) lines from src.
func stripMarkers(src string) string {
	var kept []string
	for _, line := range strings.SplitAfter(src, "\n") {
		if !strings.Contains(line, "// TODO(gotest-migrate):") {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "")
}

func readFile(t *gotest.T, dir, name string) string {
	src, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
	gotest.NoError(t, err)
	return string(src)
}

// testifyFindings lists the testify rule's findings in lint -json output.
func testifyFindings(t *gotest.T, stdout string) []string {
	var byPkg map[string]map[string][]struct {
		Posn     string `json:"posn"`
		Message  string `json:"message"`
		Category string `json:"category"`
	}
	gotest.NoError(t, json.Unmarshal([]byte(stdout), &byPkg), "stdout: %s", stdout)
	var got []string
	for _, analyzers := range byPkg {
		for _, d := range analyzers["gotestlint"] {
			if d.Category == "testify" {
				got = append(got, d.Posn+": "+d.Message)
			}
		}
	}
	return got
}
