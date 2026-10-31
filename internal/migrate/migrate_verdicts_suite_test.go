package migrate_test

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/mvrahden/go-test/internal/migrate"
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/gotestcli"
)

// MigrateVerdictsTestSuite: migrate translates only where gotest gives the
// verdict testify gave, and marks the rest with its reason.
type MigrateVerdictsTestSuite struct {
	Mod *gotestcli.TestifyModuleSharedFixture
}

func (s *MigrateVerdictsTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

// verdictHead opens every case's file; its suite holds a field.
const verdictHead = `package p

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type item struct{ n int }

type point struct{ x, y int }

type holder struct{ p *item }

var _ = assert.True

var _ = http.StatusOK

`

const verdictRunner = "\nfunc TestV(t *testing.T) { suite.Run(t, new(VSuite)) }\n"

// verdictCase is one testify file: what its migration must hold, must not
// hold, and whether the result compiles; one that does not has a marker on
// every error's line.
type verdictCase struct {
	Desc     string
	Src      string
	Want     []string
	Absent   []string
	Compiles bool
}

func (s *MigrateVerdictsTestSuite) TestMarkersAndTranslations(t *gotest.T) {
	cases := []verdictCase{
		{
			Desc: "assertions on a goroutine a go statement starts",
			Src: `type VSuite struct{ suite.Suite }

func (s *VSuite) TestA() {
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.Equal(1, 1)
		assert.True(s.T(), true)
	}()
	<-done
	s.True(true)
}
`,
			Want: []string{
				"\t\t// TODO(gotest-migrate): assertion Equal runs on a goroutine a go statement starts: gotest's assertions end that goroutine with FailNow, where testify's assert carried on and require was already unsafe — start it with gotest.Go and wait, or assert on the test's goroutine\n\t\ts.Equal(1, 1)\n",
				"\t\t// TODO(gotest-migrate): assertion True runs on a goroutine a go statement starts: gotest's assertions end that goroutine with FailNow, where testify's assert carried on and require was already unsafe — start it with gotest.Go and wait, or assert on the test's goroutine\n\t\tassert.True(t.T(), true)\n",
				"\t<-done\n\tgotest.True(t, true)\n",
			},
		},
		{
			Desc: "ElementsMatch and Subset over pointer, interface and pointer-holding elements",
			Src: `type VSuite struct{ suite.Suite }

func (s *VSuite) TestA() {
	a, b := &item{1}, &item{1}
	s.ElementsMatch([]*item{a}, []*item{b})
	s.Subset([]any{1, 2}, []any{1})
	s.ElementsMatch([]holder{{a}}, []holder{{b}})
}
`,
			Want: []string{
				"\t// TODO(gotest-migrate): testify ElementsMatch compared the *item elements deeply; gotest.ElementsMatch compares them with ==, so distinct pointers or interfaces holding equal values no longer match — compare the values by hand\n\ts.ElementsMatch(",
				"\t// TODO(gotest-migrate): testify Subset compared the any elements deeply; gotest.Subset compares them with ==, so distinct pointers or interfaces holding equal values no longer match — compare the values by hand\n\ts.Subset(",
				"\t// TODO(gotest-migrate): testify ElementsMatch compared the holder elements deeply; gotest.ElementsMatch compares them with ==, so distinct pointers or interfaces holding equal values no longer match — compare the values by hand\n\ts.ElementsMatch([]holder",
			},
		},
		{
			Desc: "ElementsMatch and Subset over elements == compares as deeply as testify",
			Src: `type VSuite struct{ suite.Suite }

func (s *VSuite) TestA() {
	s.ElementsMatch([]int{1, 2}, []int{2, 1})
	s.Subset([]point{{1, 2}, {3, 4}}, []point{{3, 4}})
	s.ElementsMatch([2]string{"a", "b"}, [2]string{"b", "a"})
}
`,
			Want: []string{
				"\tgotest.ElementsMatch(t, []int{1, 2}, []int{2, 1})\n",
				"\tgotest.Subset(t, []point{{1, 2}, {3, 4}}, []point{{3, 4}})\n",
			},
			Absent: []string{"compared the int elements", "compared the point elements"},
		},
		{
			Desc: "Zero and Empty on interface-typed values",
			Src: `type VSuite struct{ suite.Suite }

func (s *VSuite) TestA() {
	var err error
	s.Zero(err)
	var v any
	s.Empty(v)
	var p *item
	s.Zero(p)
}
`,
			Want: []string{
				"\t// TODO(gotest-migrate): testify Zero on interface type error also passes for a zero value inside it, a typed nil included; gotest.Zero passes only for nil — use gotest.Zero if nil is meant\n\ts.Zero(err)\n",
				"\t// TODO(gotest-migrate): testify Empty on interface type any judges the value inside it, so a zero number or struct counts as empty; gotest.Empty accepts only collections and pointers — convert by hand\n\ts.Empty(v)\n",
				"\tgotest.Zero(t, p)\n",
			},
		},
		{
			Desc: "Equal with an untyped constant of another type than the value",
			Src: `type VSuite struct{ suite.Suite }

func (s *VSuite) TestA() {
	var x int64 = 5
	s.NotEqual(5, x)
	var p *item
	s.NotEqual(nil, p)
	s.NotEqual(1, 1.0)
}
`,
			Want: []string{
				"\t// TODO(gotest-migrate): testify NotEqual compared 5 as int with a value of type int64, types it never finds equal; gotest.NotEqual converts 5 to int64 and compares the values — convert by hand\n\ts.NotEqual(5, x)\n",
				"\t// TODO(gotest-migrate): testify NotEqual compared nil with a value of type *item held in an interface, which is never nil; gotest.NotEqual compares that *item itself with nil — convert by hand\n\ts.NotEqual(nil, p)\n",
				"\t// TODO(gotest-migrate): testify NotEqual compared 1 as int with a value of type float64, types it never finds equal; gotest.NotEqual converts 1 to float64 and compares the values — convert by hand\n\ts.NotEqual(1, 1.0)\n",
			},
		},
		{
			Desc: "Equal with an untyped constant the value's type takes as testify did",
			Src: `type VSuite struct{ suite.Suite }

func (s *VSuite) TestA() {
	var y int
	s.Equal(0, y)
	var d any = 5
	s.Equal(5, d)
	s.Equal("a", "a")
	var f float64 = 1
	s.Equal(1.0, f)
}
`,
			Want: []string{
				"\tgotest.Equal(t, 0, y)\n",
				"\tgotest.Equal(t, 5, d)\n",
				"\tgotest.Equal(t, \"a\", \"a\")\n",
				"\tgotest.Equal(t, 1.0, f)\n",
			},
			Absent:   []string{"TODO(gotest-migrate)"},
			Compiles: true,
		},
		{
			Desc: "Equal and NotEqual with an untyped constant of another package",
			Src: `type VSuite struct{ suite.Suite }

func (s *VSuite) TestA() {
	var port uint16 = 8080
	s.NotEqual(http.StatusOK, port)
	s.Equal(http.StatusOK, 200)
}
`,
			Want: []string{
				"\t// TODO(gotest-migrate): testify NotEqual compared http.StatusOK as int with a value of type uint16, types it never finds equal",
				"\tgotest.Equal(t, http.StatusOK, 200)\n",
			},
		},
		{
			Desc: "Equal and NotEqual of two concrete types",
			Src: `type IDs []int

type VSuite struct{ suite.Suite }

func (s *VSuite) TestA() {
	ids := IDs{1}
	s.NotEqual([]int{1}, ids)
	var err error
	s.Equal(err, (*myErr)(nil))
}

type myErr struct{}

func (*myErr) Error() string { return "" }
`,
			Want: []string{
				"\t// TODO(gotest-migrate): testify NotEqual compared a []int with a IDs, types it never finds equal; gotest.NotEqual would convert one to the other and compare the values — convert by hand\n\ts.NotEqual([]int{1}, ids)\n",
				"\tgotest.Equal[error](t, err, (*myErr)(nil))\n",
			},
		},
		{
			Desc: "Len on a pointer to an array",
			Src: `type VSuite struct{ suite.Suite }

func (s *VSuite) TestA() {
	var p *[3]int
	s.Len(p, 3)
	s.Len([]int{1}, 1)
}
`,
			Want: []string{
				"\t// TODO(gotest-migrate): testify Len on *[3]int counted the array's length, nil pointer or not; gotest.Len takes no pointer — assert on len(p) with gotest.Equal\n\ts.Len(p, 3)\n",
				"\tgotest.Len(t, []int{1}, 1)\n",
			},
		},
		{
			Desc: "InDelta where a NaN can decide",
			Src: `type VSuite struct{ suite.Suite }

func (s *VSuite) TestA() {
	want, got, eps := 1.0, 1.0, 0.1
	s.InDelta(want, got, 0.1)
	s.InDelta(1.0, got, eps)
	s.InDelta(1.0, got, 0.1)
	n, m := 1, 2
	s.InDelta(n, m, 1)
}
`,
			Want: []string{
				"\t// TODO(gotest-migrate): testify InDelta passed when both values were NaN; gotest.InDelta fails then",
				"\t// TODO(gotest-migrate): testify InDelta passed any difference within a NaN delta; gotest.InDelta fails a NaN delta — convert by hand\n\ts.InDelta(1.0, got, eps)\n",
				"\tgotest.InDelta(t, 1.0, got, 0.1)\n",
				"\tgotest.InDelta(t, n, m, 1)\n",
			},
		},
		{
			Desc: "TearDownSuite beside SetupSuite",
			Src: `type VSuite struct {
	suite.Suite
	db *item
}

func (s *VSuite) SetupSuite() {
	s.Require().NotZero(1)
	s.db = &item{1}
}

func (s *VSuite) TearDownSuite() { s.db.n = 0 }

func (s *VSuite) TestA() {}
`,
			Want: []string{
				"// TODO(gotest-migrate): testify skipped TearDownSuite when SetupSuite stopped early, failed, skipped or panicked; gotest runs AfterAll even then, on whatever BeforeAll left half set up — guard what it releases\nfunc (s *VTestSuite) AfterAll(t *gotest.T) { s.db.n = 0 }\n",
			},
		},
		{
			Desc: "TearDownSuite beside a SetupSuite that cannot stop early",
			Src: `type VSuite struct {
	suite.Suite
	db *item
}

func (s *VSuite) SetupSuite() { s.db = &item{1} }

func (s *VSuite) TearDownSuite() { s.db.n = 0 }

func (s *VSuite) TestA() {}
`,
			Want:     []string{"func (s *VTestSuite) AfterAll(t *gotest.T) { s.db.n = 0 }\n"},
			Absent:   []string{"TODO(gotest-migrate)"},
			Compiles: true,
		},
		{
			Desc: "TearDownSuite alone",
			Src: `type VSuite struct {
	suite.Suite
	db *item
}

func (s *VSuite) TearDownSuite() { s.db = nil }

func (s *VSuite) TestA() {}
`,
			Want:     []string{"func (s *VTestSuite) AfterAll(t *gotest.T) { s.db = nil }\n"},
			Absent:   []string{"TODO(gotest-migrate)"},
			Compiles: true,
		},
		{
			Desc: "TearDownSuite beside a SetupSuite that stops through a helper handed its T",
			Src: `type VSuite struct {
	suite.Suite
	db *item
}

func requireDocker(t *testing.T) {
	if t != nil {
		t.Skip("no docker")
	}
}

func (s *VSuite) SetupSuite() {
	requireDocker(s.T())
	s.db = &item{1}
}

func (s *VSuite) TearDownSuite() { s.db.n = 0 }

func (s *VSuite) TestA() {}
`,
			Want: []string{
				"// TODO(gotest-migrate): testify skipped TearDownSuite when SetupSuite stopped early, failed, skipped or panicked; gotest runs AfterAll even then, on whatever BeforeAll left half set up — guard what it releases\nfunc (s *VTestSuite) AfterAll(t *gotest.T) { s.db.n = 0 }\n",
			},
		},
		{
			Desc: "TearDownSuite beside a SetupSuite that stops through a suite method",
			Src: `type VSuite struct {
	suite.Suite
	db *item
}

func (s *VSuite) open() *item {
	s.Require().NotZero(1)
	return &item{1}
}

func (s *VSuite) SetupSuite() { s.db = s.open() }

func (s *VSuite) TearDownSuite() { s.db.n = 0 }

func (s *VSuite) TestA() {}
`,
			Want: []string{
				"// TODO(gotest-migrate): testify skipped TearDownSuite when SetupSuite stopped early",
			},
		},
		{
			Desc: "TearDownSuite beside a SetupSuite whose helpers cannot stop",
			Src: `type VSuite struct {
	suite.Suite
	db *item
}

func logStart(t *testing.T) { t.Log("start") }

func (s *VSuite) SetupSuite() {
	logStart(s.T())
	s.db = &item{1}
}

func (s *VSuite) TearDownSuite() { s.db.n = 0 }

func (s *VSuite) TestA() {}
`,
			Want:     []string{"func (s *VTestSuite) AfterAll(t *gotest.T) { s.db.n = 0 }\n"},
			Absent:   []string{"TODO(gotest-migrate)"},
			Compiles: true,
		},
		{
			Desc: "TearDownSuite beside a base's SetupSuite that can stop early",
			Src: `type BaseSuite struct {
	suite.Suite
}

func (b *BaseSuite) SetupSuite() { b.Require().NotZero(1) }

type VSuite struct {
	BaseSuite
	db *item
}

func (s *VSuite) TearDownSuite() { s.db.n = 0 }

func (s *VSuite) TestA() {}
`,
			Want: []string{
				"// TODO(gotest-migrate): testify skipped TearDownSuite when SetupSuite stopped early, failed, skipped or panicked; gotest runs AfterAll even then, on whatever BeforeAll left half set up — guard what it releases\nfunc (s *VTestSuite) AfterAll(t *gotest.T) { s.db.n = 0 }\n",
			},
		},
		{
			Desc: "a base's TearDownSuite beside a SetupSuite that can stop early",
			Src: `type BaseSuite struct {
	suite.Suite
	db *item
}

func (b *BaseSuite) TearDownSuite() { b.db.n = 0 }

type VSuite struct {
	BaseSuite
}

func (s *VSuite) SetupSuite() {
	s.Require().NotZero(1)
	s.db = &item{1}
}

func (s *VSuite) TestA() {}
`,
			Want: []string{
				"// TODO(gotest-migrate): testify skipped the TearDownSuite VTestSuite has from its base field when SetupSuite stopped early, failed, skipped or panicked; gotest runs the AfterAll forwarding it even then, on whatever BeforeAll left half set up — guard what it releases\ntype VTestSuite struct {\n",
			},
		},
		{
			Desc: "s.T().Parallel() and s.T().Cleanup",
			Src: `type VSuite struct{ suite.Suite }

func (s *VSuite) SetupSuite() {
	s.T().Cleanup(func() {})
}

func (s *VSuite) TestA() {
	s.T().Parallel()
	s.T().Cleanup(func() {})
}
`,
			Want: []string{
				"\t// TODO(gotest-migrate): t.T().Cleanup in a gotest suite — release the resource in AfterEach, or in AfterAll when BeforeAll acquired it, and delete this call\n\tt.T().Cleanup(func() {})\n}\n\nfunc (s *VTestSuite) TestA",
				"\t// TODO(gotest-migrate): t.T().Parallel() in a gotest suite — set Parallel in the suite's SuiteConfig instead, with a BeforeEach returning per-test state, and delete this call\n\tt.T().Parallel()\n\t// TODO(gotest-migrate): t.T().Cleanup in a gotest suite — release the resource in AfterEach, or in AfterAll when BeforeAll acquired it, and delete this call\n\tt.T().Cleanup(func() {})\n}\n",
			},
		},
		{
			Desc: "a suite with fields whose tests gotest runs in another order",
			Src: `type VSuite struct {
	suite.Suite
	n int
}

func (s *VSuite) TestB() { s.n++ }

func (s *VSuite) TestA() { s.Equal(0, s.n) }
`,
			Want: []string{
				"// TODO(gotest-migrate): gotest runs VTestSuite's tests in source order, testify ran them in name order: TestB now runs before TestA, and they share the suite's fields — declare the tests in name order if one depends on another\ntype VTestSuite struct {\n",
			},
		},
		{
			Desc: "a child whose base's tests sort between its own",
			Src: `type BaseSuite struct {
	suite.Suite
	n int
}

func (b *BaseSuite) TestB() {}

type VSuite struct{ BaseSuite }

func (s *VSuite) TestA() {}

func (s *VSuite) TestC() {}
`,
			Want:     []string{"func (s *VTestSuite) TestA(t *gotest.T) {}\n\nfunc (s *VTestSuite) TestB(t *gotest.T) {\n\ts.base.TestB(t)\n}\n\nfunc (s *VTestSuite) TestC(t *gotest.T) {}\n"},
			Absent:   []string{"TODO(gotest-migrate)"},
			Compiles: true,
		},
		{
			Desc: "a suite without fields whose tests gotest runs in another order",
			Src: `type VSuite struct{ suite.Suite }

func (s *VSuite) TestB() {}

func (s *VSuite) TestA() {}
`,
			Absent:   []string{"TODO(gotest-migrate)"},
			Compiles: true,
		},
		{
			Desc: "a suite with fields whose tests are declared in name order",
			Src: `type VSuite struct {
	suite.Suite
	n int
}

func (s *VSuite) TestA() { s.n++ }

func (s *VSuite) TestB() { s.Equal(1, s.n) }
`,
			Absent:   []string{"TODO(gotest-migrate)"},
			Compiles: true,
		},
	}
	for t, c := range gotest.Each(t, cases) { //nolint:gocritic // rangeValCopy: a literal table
		run := migrateModule(t, s.Mod, migrate.Options{}, map[string]string{"p/a_test.go": verdictHead + c.Src + verdictRunner}, "p")
		out := run.files["p/a_test.go"]
		for _, want := range c.Want {
			gotest.Contains(t, out, want)
		}
		for _, absent := range c.Absent {
			gotest.NotContains(t, out, absent)
		}
		if c.Compiles {
			gotest.Empty(t, compiles(t, run.dir))
			continue
		}
		gotest.Empty(t, unmarkedErrors(t, run.dir), "out:\n%s", out)
	}
}

// importsSrc spreads the testify imports between imports that stay.
const importsSrc = `package p

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"os"
	"testing"

	"example.com/m/x"
	"github.com/stretchr/testify/assert"
	"example.com/m/y"
)

type ISuite struct{ suite.Suite }

func (s *ISuite) TestA() {
	require.True(s.T(), os.Getpid() > 0)
	assert.Equal(s.T(), x.V, y.V)
	fmt.Println()
}

func TestI(t *testing.T) { suite.Run(t, new(ISuite)) }
`

func (s *MigrateVerdictsTestSuite) TestImportBlock(t *gotest.T) {
	t.When("testify imports sit between imports that stay", func(w *gotest.T) {
		run := migrateModule(w, s.Mod, migrate.Options{}, map[string]string{
			"x/x.go":      "package x\n\nconst V = 1\n",
			"y/y.go":      "package y\n\nconst V = 1\n",
			"p/a_test.go": importsSrc,
		}, "p")

		w.It("leaves no blank line where they stood", func(it *gotest.T) {
			gotest.Contains(it, run.files["p/a_test.go"], "import (\n\t\"fmt\"\n\t\"os\"\n\n\t\"example.com/m/x\"\n\t\"example.com/m/y\"\n\t\"github.com/mvrahden/go-test/pkg/gotest\"\n)\n")
			gotest.Empty(it, compiles(it, run.dir))
		})
	})
}

// MigrateModuleFilesTestSuite: migrate never edits go.mod or go.sum. It
// sets GOFLAGS, so it runs apart from the parallel suites.
type MigrateModuleFilesTestSuite struct {
	Mod *gotestcli.TestifyModuleSharedFixture
}

func (s *MigrateModuleFilesTestSuite) TestGoSumMissingAnEntry(t *gotest.T) {
	t.When("go.sum lacks an entry the load needs and GOFLAGS asks for -mod=mod", func(w *gotest.T) {
		root := w.TempDir()
		gotest.NoError(w, s.Mod.Write(root, map[string]string{"p/a_test.go": `package p

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type PSuite struct{ suite.Suite }

func (s *PSuite) TestA() { s.True(true) }

func TestP(t *testing.T) { suite.Run(t, new(PSuite)) }
`}))
		var kept []string
		for line := range strings.Lines(s.Mod.GoSum) {
			if !strings.Contains(line, "go-difflib") {
				kept = append(kept, line)
			}
		}
		sum := strings.Join(kept, "")
		gotest.NoError(w, os.WriteFile(filepath.Join(root, "go.sum"), []byte(sum), 0o600))
		w.Setenv("GOFLAGS", "-mod=mod")
		w.Setenv("GOWORK", "off")
		_, _ = migrate.MigratePackages([]string{filepath.Join(root, "p")}, migrate.Options{})

		w.It("writes neither go.mod nor go.sum", func(it *gotest.T) {
			gotSum, err := os.ReadFile(filepath.Join(root, "go.sum"))
			gotest.NoError(it, err)
			gotest.Equal(it, sum, string(gotSum))
			gotMod, err := os.ReadFile(filepath.Join(root, "go.mod"))
			gotest.NoError(it, err)
			gotest.Equal(it, s.Mod.GoMod, string(gotMod))
		})

	})
}
