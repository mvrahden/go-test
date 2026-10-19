package migrate_test

import (
	"encoding/json"
	"os"
	"path/filepath"

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

func (s *MigrateBaseSuitesTestSuite) TestBaseOutsideTheMigratedFiles(t *gotest.T) {
	t.When("a testify suite embeds a base suite declared outside the migrated test files", func(w *gotest.T) {
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
