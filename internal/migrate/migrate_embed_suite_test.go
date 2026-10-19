package migrate_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/gotestcli"
)

// MigrateEmbedTestSuite migrates testify suites built on a base suite: the
// child holds the base in a named field and forwards its hooks and tests,
// the base becomes a helper type, and the result compiles, runs and leaves
// no testify behind.
type MigrateEmbedTestSuite struct {
	Mod *gotestcli.TestifyModuleSharedFixture
	CLI *gotestcli.BinarySharedFixture
}

func (s *MigrateEmbedTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

const embedHooksSrc = `package p

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
)

type BaseSuite struct {
	suite.Suite
	n int
}

func (b *BaseSuite) SetupSuite()    { fmt.Println("RAN base SetupSuite") }
func (b *BaseSuite) TearDownSuite() { fmt.Println("RAN base TearDownSuite") }
func (b *BaseSuite) SetupTest()     { b.n = 1; fmt.Println("RAN base SetupTest") }
func (b *BaseSuite) TearDownTest()  { fmt.Println("RAN base TearDownTest") }

type ChildSuite struct {
	BaseSuite
}

func (s *ChildSuite) TestOwn() {
	s.Equal(1, s.n)
	fmt.Println("RAN child TestOwn")
}

func TestChild(t *testing.T) { suite.Run(t, new(ChildSuite)) }
`

const embedTestsSrc = `package p

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
)

type BaseSuite struct{ suite.Suite }

func (b *BaseSuite) TestShared() {
	b.True(true)
	fmt.Println("RAN base TestShared")
}

func (b *BaseSuite) TestOverridden() { fmt.Println("RAN base TestOverridden") }

type ChildSuite struct{ BaseSuite }

func (s *ChildSuite) TestOverridden() {
	s.Require().True(true)
	fmt.Println("RAN child TestOverridden")
}

func TestChild(t *testing.T) { suite.Run(t, new(ChildSuite)) }
`

const embedSelectorSrc = `package p

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
)

type BaseSuite struct {
	suite.Suite
	n int
}

func (b *BaseSuite) value() int { return b.n + 1 }

func (b *BaseSuite) check(v int) {
	b.T().Log("checking")
	b.Require().Equal(3, v)
}

type ChildSuite struct {
	BaseSuite
}

func (s *ChildSuite) TestSelectors() {
	s.n = 2
	s.BaseSuite.n++
	s.n--
	s.check(s.value())
	fmt.Println("RAN child TestSelectors", s.n)
}

func TestChild(t *testing.T) { suite.Run(t, new(ChildSuite)) }
`

const embedLiteralSrc = `package p

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
)

type BaseSuite struct {
	suite.Suite
	n int
}

type ChildSuite struct {
	BaseSuite
	m int
}

func seeded() *ChildSuite { return &ChildSuite{BaseSuite: BaseSuite{n: 4}, m: 5} }

func (s *ChildSuite) TestLiteral() {
	c := seeded()
	s.Equal(9, c.n+c.m)
	fmt.Println("RAN child TestLiteral")
}

func TestChild(t *testing.T) { suite.Run(t, new(ChildSuite)) }
`

const embedHookCallSrc = `package p

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
)

type BaseSuite struct {
	suite.Suite
	n int
}

func (b *BaseSuite) SetupTest() { b.n = 1; fmt.Println("RAN base SetupTest") }

type ChildSuite struct {
	BaseSuite
	m int
}

func (s *ChildSuite) SetupTest() {
	s.BaseSuite.SetupTest()
	s.m = s.n + 1
}

func (s *ChildSuite) TestHook() {
	s.Equal(2, s.m)
	fmt.Println("RAN child TestHook")
}

func TestChild(t *testing.T) { suite.Run(t, new(ChildSuite)) }
`

const embedNamedBaseSrc = `package p

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
)

type BaseTestSuite struct{ suite.Suite }

func (b *BaseTestSuite) TestShared() { fmt.Println("RAN base TestShared") }

type ChildSuite struct{ BaseTestSuite }

func (s *ChildSuite) TestOwn() { fmt.Println("RAN child TestOwn") }

func TestChild(t *testing.T) { suite.Run(t, new(ChildSuite)) }
`

const embedNamedBaseRefSrc = `package p

func newBase() *BaseTestSuite { return &BaseTestSuite{} }

var _ = newBase
`

const embedForeignBaseSrc = `package testutil

import "github.com/stretchr/testify/suite"

type BaseSuite struct{ suite.Suite }

func (b *BaseSuite) SetupSuite() {}
`

const embedForeignChildSrc = `package api

import (
	"testing"

	"example.com/m/testutil"
	"github.com/stretchr/testify/suite"
)

type APISuite struct{ testutil.BaseSuite }

func (s *APISuite) TestA() { s.Require().True(true) }

func TestAPI(t *testing.T) { suite.Run(t, new(APISuite)) }
`

const embedChainSrc = `package p

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
)

type RootSuite struct {
	suite.Suite
	n int
}

func (r *RootSuite) SetupTest() { r.n = 1; fmt.Println("RAN root SetupTest") }
func (r *RootSuite) TestRoot()  { r.Equal(1, r.n); fmt.Println("RAN root TestRoot") }

type MidSuite struct {
	RootSuite
}

func (m *MidSuite) SetupSuite() { fmt.Println("RAN mid SetupSuite") }
func (m *MidSuite) TestMid()    { m.Equal(1, m.n); fmt.Println("RAN mid TestMid") }

type LeafSuite struct {
	MidSuite
}

func (s *LeafSuite) TestLeaf() {
	s.Equal(1, s.n)
	s.RootSuite.n = 2
	fmt.Println("RAN leaf TestLeaf", s.MidSuite.RootSuite.n)
}

func TestLeaf(t *testing.T) { suite.Run(t, new(LeafSuite)) }
`

// embedShape is one way a testify suite builds on a base suite.
type embedShape struct {
	Desc  string
	Files map[string]string
	// Exit is migrate's exit code; Stderr is a line it must print.
	Exit   int
	Stderr string
	// Want lists, by file, text the migrated file must hold.
	Want map[string][]string
	// Gone lists text no migrated file may hold.
	Gone []string
	// Ran lists lines the migrated tests print; Skipped those they must not.
	Ran     []string
	Skipped []string
}

func (s *MigrateEmbedTestSuite) TestBaseShapes(t *gotest.T) {
	shapes := []embedShape{
		{
			Desc:  "a base with hooks",
			Files: map[string]string{"p/a_test.go": embedHooksSrc},
			Want: map[string][]string{"p/a_test.go": {
				"type BaseSuite struct {\n\tn int\n}",
				"func (b *BaseSuite) BeforeAll(t *gotest.T)",
				"func (b *BaseSuite) BeforeEach(t *gotest.T) { b.n = 1; fmt.Println(\"RAN base SetupTest\") }",
				"type ChildTestSuite struct {\n\tbase BaseSuite\n}",
				"func (s *ChildTestSuite) BeforeAll(t *gotest.T) {\n\ts.base.BeforeAll(t)\n}",
				"func (s *ChildTestSuite) AfterAll(t *gotest.T) {\n\ts.base.AfterAll(t)\n}",
				"func (s *ChildTestSuite) BeforeEach(t *gotest.T) {\n\ts.base.BeforeEach(t)\n}",
				"func (s *ChildTestSuite) AfterEach(t *gotest.T) {\n\ts.base.AfterEach(t)\n}",
				"gotest.Equal(t, 1, s.base.n)",
			}},
			Ran: []string{"RAN base SetupSuite", "RAN base SetupTest", "RAN child TestOwn", "RAN base TearDownTest", "RAN base TearDownSuite"},
		},
		{
			Desc:  "a base with tests the child overrides and some it does not",
			Files: map[string]string{"p/a_test.go": embedTestsSrc},
			Want: map[string][]string{"p/a_test.go": {
				"func (b *BaseSuite) TestShared(t *gotest.T) {\n\tgotest.True(t, true)",
				"func (s *ChildTestSuite) TestShared(t *gotest.T) {\n\ts.base.TestShared(t)\n}",
				"func (s *ChildTestSuite) TestOverridden(t *gotest.T) {\n\tgotest.True(t, true)",
			}},
			Gone:    []string{"s.base.TestOverridden"},
			Ran:     []string{"RAN base TestShared", "RAN child TestOverridden"},
			Skipped: []string{"RAN base TestOverridden"},
		},
		{
			Desc:  "a selector through the embedding",
			Files: map[string]string{"p/a_test.go": embedSelectorSrc},
			Want: map[string][]string{"p/a_test.go": {
				"func (b *BaseSuite) check(t *gotest.T, v int) {\n\tt.T().Log(\"checking\")\n\tgotest.Equal(t, 3, v)\n}",
				"func (b *BaseSuite) value() int { return b.n + 1 }",
				"\ts.base.n = 2\n\ts.base.n++\n\ts.base.n--\n\ts.base.check(t, s.base.value())\n",
			}},
			Ran: []string{"RAN child TestSelectors 2"},
		},
		{
			Desc:  "a keyed literal",
			Files: map[string]string{"p/a_test.go": embedLiteralSrc},
			Want: map[string][]string{"p/a_test.go": {
				"return &ChildTestSuite{base: BaseSuite{n: 4}, m: 5}",
				"gotest.Equal(t, 9, c.base.n+c.m)",
			}},
			Ran: []string{"RAN child TestLiteral"},
		},
		{
			Desc:  "an explicit base hook call",
			Files: map[string]string{"p/a_test.go": embedHookCallSrc},
			Want: map[string][]string{"p/a_test.go": {
				"func (s *ChildTestSuite) BeforeEach(t *gotest.T) {\n\ts.base.BeforeEach(t)\n\ts.m = s.base.n + 1\n}",
			}},
			Ran: []string{"RAN base SetupTest", "RAN child TestHook"},
		},
		{
			Desc: "a base named like a suite that no runner ran",
			Files: map[string]string{
				"p/a_test.go":   embedNamedBaseSrc,
				"p/ref_test.go": embedNamedBaseRefSrc,
			},
			Want: map[string][]string{
				"p/a_test.go": {
					"type BaseTestSuiteHelpers struct{}",
					"type ChildTestSuite struct{ base BaseTestSuiteHelpers }",
				},
				"p/ref_test.go": {"func newBase() *BaseTestSuiteHelpers { return &BaseTestSuiteHelpers{} }"},
			},
			Ran: []string{"RAN base TestShared", "RAN child TestOwn"},
		},
		{
			Desc: "a base in a package outside the migrated test files",
			Files: map[string]string{
				"testutil/base.go": embedForeignBaseSrc,
				"api/a_test.go":    embedForeignChildSrc,
			},
			Exit:   2,
			Stderr: "APISuite embeds testutil.BaseSuite, a testify suite declared outside the test files being migrated, in package example.com/m/testutil",
		},
		{
			Desc:  "a two-level chain",
			Files: map[string]string{"p/a_test.go": embedChainSrc},
			Want: map[string][]string{"p/a_test.go": {
				"type MidSuite struct {\n\tbase RootSuite\n}",
				"type LeafTestSuite struct {\n\tbase MidSuite\n}",
				"func (m *MidSuite) BeforeEach(t *gotest.T) {\n\tm.base.BeforeEach(t)\n}",
				"func (s *LeafTestSuite) BeforeEach(t *gotest.T) {\n\ts.base.BeforeEach(t)\n}",
				"func (s *LeafTestSuite) TestRoot(t *gotest.T) {\n\ts.base.TestRoot(t)\n}",
				"func (s *LeafTestSuite) TestMid(t *gotest.T) {\n\ts.base.TestMid(t)\n}",
				"gotest.Equal(t, 1, s.base.base.n)\n\ts.base.base.n = 2\n\tfmt.Println(\"RAN leaf TestLeaf\", s.base.base.n)",
			}},
			Ran: []string{"RAN mid SetupSuite", "RAN root SetupTest", "RAN root TestRoot", "RAN mid TestMid", "RAN leaf TestLeaf 2"},
		},
	}
	for t, shape := range gotest.Each(t, shapes) { //nolint:gocritic // rangeValCopy: intentional
		dir := t.TempDir()
		gotest.NoError(t, s.Mod.Write(dir, shape.Files))
		stdout, stderr, code := s.runCLI(t, dir, "migrate", "./...")
		gotest.Equal(t, shape.Exit, code, "stdout: %s\nstderr: %s", stdout, stderr)
		files := readFiles(t, dir, shape.Files)
		if shape.Exit != 0 {
			gotest.Contains(t, stderr, shape.Stderr)
			for name, src := range shape.Files {
				gotest.Equal(t, src, files[name], "a refused run writes nothing")
			}
			continue
		}
		for _, name := range sortedKeys(files) {
			gotest.NotContains(t, files[name], "TODO(gotest-migrate)", "%s:\n%s", name, files[name])
			for _, want := range shape.Want[name] {
				gotest.Contains(t, files[name], want, "%s:\n%s", name, files[name])
			}
			for _, gone := range shape.Gone {
				gotest.NotContains(t, files[name], gone, "%s:\n%s", name, files[name])
			}
			gotest.NotContains(t, files[name], "github.com/stretchr/testify", "%s:\n%s", name, files[name])
		}

		vetOut, vetErr, err := goCmd(t, dir, "vet", "./...")
		gotest.NoError(t, err, "go vet:\n%s%s", vetOut, vetErr)
		gotest.Empty(t, s.testifyFindings(t, dir))

		runOut, runErr, runCode := s.runCLI(t, dir, "-v", "./...")
		gotest.Equal(t, 0, runCode, "stdout: %s\nstderr: %s", runOut, runErr)
		for _, line := range shape.Ran {
			gotest.Contains(t, runOut, line)
		}
		for _, line := range shape.Skipped {
			gotest.NotContains(t, runOut, line)
		}
		for _, name := range sortedKeys(files) {
			gotest.MatchSnapshot(t, files[name], name)
		}
	}
}

// runCLI runs the gotest under test in dir.
func (s *MigrateEmbedTestSuite) runCLI(t *gotest.T, dir string, args ...string) (stdout, stderr string, code int) {
	return runBinary(t, s.CLI.Binary, dir, args...)
}

// runBinary runs the CLI binary in dir, stdout and stderr apart.
func runBinary(t *gotest.T, binary, dir string, args ...string) (stdout, stderr string, code int) {
	testkit.ScrubActionsEnv()
	cmd := exec.CommandContext(t.Context(), binary, args...) //nolint:gosec // G204: the CLI under test
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=", "PWD="+dir)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	_ = cmd.Run()
	return out.String(), errb.String(), cmd.ProcessState.ExitCode()
}

// testifyFindings lists what gotest lint's testify rule reports in dir.
func (s *MigrateEmbedTestSuite) testifyFindings(t *gotest.T, dir string) []string {
	stdout, stderr, _ := s.runCLI(t, dir, "lint", "-json", "./...")
	var byPkg map[string]map[string][]struct {
		Posn     string `json:"posn"`
		Message  string `json:"message"`
		Category string `json:"category"`
	}
	gotest.NoError(t, json.Unmarshal([]byte(stdout), &byPkg), "stdout: %s\nstderr: %s", stdout, stderr)
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

// readFiles reads names back from dir, by slash path.
func readFiles(t *gotest.T, dir string, names map[string]string) map[string]string {
	out := map[string]string{}
	for name := range names {
		if !strings.HasSuffix(name, ".go") {
			continue
		}
		got, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
		gotest.NoError(t, err)
		out[name] = string(got)
	}
	return out
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
