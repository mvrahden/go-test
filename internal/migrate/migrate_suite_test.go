package migrate_test

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/migrate"
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/gotestcli"
)

// todoInputSrc is a testify suite containing lifecycle hooks and assertion
// calls the migrator cannot convert. The migrator must annotate every one of
// them with a TODO(gotest-migrate) marker instead of silently skipping them.
const todoInputSrc = `package sample

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type SampleSuite struct {
	suite.Suite
}

func (s *SampleSuite) SetupTest() {}

func (s *SampleSuite) SetupSubTest() {}

func (s *SampleSuite) TearDownSubTest() {}

func (s *SampleSuite) BeforeTest(suiteName, testName string) {}

func (s *SampleSuite) AfterTest(suiteName, testName string) {}

func (s *SampleSuite) HandleStats(suiteName string, stats *suite.SuiteInformation) {}

func (s *SampleSuite) TestThings() {
	var target error
	s.Require().ErrorAs(nil, &target)
	s.Assert().Eventually(func() bool { return true }, 0, 0)
	s.ElementsMatch([]int{1}, []int{1})
	assert.InDelta(s.T(), 1.0, 1.0, 0.1)
	require.Regexp(s.T(), "a", "a")
	s.Require().NoError(nil)
	s.Assert().Equal(1, 1)
	s.Equal(2, 2)
}

func TestSampleSuite(t *testing.T) {
	suite.Run(t, new(SampleSuite))
}
`

// MigrateTestSuite tests the testify-to-gotest migration, focusing on
// TODO(gotest-migrate) markers for constructs the migrator cannot convert.
type MigrateTestSuite struct {
	Mod *gotestcli.TestifyModuleSharedFixture
}

func (s *MigrateTestSuite) SuiteConfig() gotest.SuiteConfig {
	// Estimated for Windows CI: slowest method ~115s; the budget is twice that, rounded up.
	return gotest.SuiteConfig{Parallel: true, Timeout: 4 * time.Minute}
}

// migrateSource migrates src as the only test file of a package and
// returns the result.
func (s *MigrateTestSuite) migrateSource(t *gotest.T, src string) string {
	run := s.migrateFile(t, src)
	gotest.Len(t, run.results, 1)
	return run.files["sample/sample_test.go"]
}

func (s *MigrateTestSuite) migrateFile(t *gotest.T, src string) pkgRun {
	return migrateModule(t, s.Mod, migrate.Options{}, map[string]string{"sample/sample_test.go": src}, "sample")
}

// rerun migrates the package of run again.
func rerun(t *gotest.T, run pkgRun) []migrate.MigrateResult {
	results, err := migrate.MigratePackages([]string{filepath.Join(run.dir, "sample")}, migrate.Options{})
	gotest.NoError(t, err)
	return results
}

func (s *MigrateTestSuite) TestTodoMarkers(t *gotest.T) {
	t.When("a suite has unconvertible hooks and assertions", func(w *gotest.T) {
		out := s.migrateSource(w, todoInputSrc)

		w.It("annotates every unsupported lifecycle hook", func(it *gotest.T) {
			for _, hook := range []string{
				"SetupSubTest", "TearDownSubTest", "BeforeTest", "AfterTest", "HandleStats",
			} {
				gotest.Contains(it, out,
					"// TODO(gotest-migrate): unsupported testify hook "+hook+" — convert manually")
			}
		})

		w.It("annotates every unmapped assertion call", func(it *gotest.T) {
			for _, name := range []string{"ErrorAs", "Eventually"} {
				gotest.Contains(it, out,
					"// TODO(gotest-migrate): unmapped assertion "+name+" — convert manually")
			}
		})

		w.It("places hook markers directly above the method declaration", func(it *gotest.T) {
			gotest.Contains(it, out,
				"// TODO(gotest-migrate): unsupported testify hook SetupSubTest — convert manually\nfunc (s *SampleTestSuite) SetupSubTest() {}")
		})

		w.It("places assertion markers directly above the statement", func(it *gotest.T) {
			gotest.Contains(it, out,
				"// TODO(gotest-migrate): unmapped assertion ErrorAs — convert manually\n\ts.Require().ErrorAs(nil, &target)")
		})

		w.It("converts the assertions gotest has, the suite's own included", func(it *gotest.T) {
			gotest.Contains(it, out, "\tgotest.ElementsMatch(t, []int{1}, []int{1})\n")
			gotest.Contains(it, out, "\tgotest.InDelta(t, 1.0, 1.0, 0.1)\n")
			gotest.Contains(it, out, "\tgotest.Regexp(t, \"a\", \"a\")\n")
			gotest.Contains(it, out, "\tgotest.Equal(t, 2, 2)\n")
		})

		w.It("still converts mapped assertions", func(it *gotest.T) {
			gotest.Contains(it, out, "gotest.NoError(t, nil)")
			gotest.Contains(it, out, "gotest.Equal(t, 1, 1)")
		})

		w.It("does not annotate mapped assertions", func(it *gotest.T) {
			gotest.NotContains(it, out, "unmapped assertion NoError")
			gotest.NotContains(it, out, "unmapped assertion Equal")
		})

		w.It("converts supported hooks untouched by markers", func(it *gotest.T) {
			gotest.Contains(it, out, "func (s *SampleTestSuite) BeforeEach(t *gotest.T) {}")
			gotest.NotContains(it, out, "unsupported testify hook SetupTest")
		})

		w.It("emits exactly one marker per finding", func(it *gotest.T) {
			gotest.Equal(it, 7, strings.Count(out, "TODO(gotest-migrate)"))
		})

		w.It("produces parseable, gofmt-clean output", func(it *gotest.T) {
			fset := token.NewFileSet()
			_, err := parser.ParseFile(fset, "sample_test.go", out, parser.ParseComments)
			gotest.NoError(it, err)
		})
	})

	t.When("a suite is fully convertible", func(w *gotest.T) {
		const cleanSrc = `package sample

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type CleanSuite struct {
	suite.Suite
}

func (s *CleanSuite) TestOK() {
	s.Require().Equal(1, 1)
}

func TestCleanSuite(t *testing.T) {
	suite.Run(t, new(CleanSuite))
}
`
		out := s.migrateSource(w, cleanSrc)

		w.It("emits no TODO markers", func(it *gotest.T) {
			gotest.NotContains(it, out, "TODO(gotest-migrate)")
		})
	})
}

// scopedSrc mixes a test method, which gains t, with a helper that does not.
const scopedSrc = `package sample

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type ScopedSuite struct {
	suite.Suite
}

func (s *ScopedSuite) helper() {
	s.T().Helper()
}

func (s *ScopedSuite) TestUsesT() {
	logTo(s.T())
	s.Run("sub", func() {
		s.Require().Equal(1, 1)
	})
}

func logTo(t *testing.T) {}

func TestScopedSuite(t *testing.T) {
	suite.Run(t, new(ScopedSuite))
}
`

// indirectSrc embeds one suite in another: the base would be renamed and the
// child left behind, so the file is refused whole.
const indirectSrc = `package sample

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type BaseSuite struct {
	suite.Suite
}

type OrderSuite struct {
	BaseSuite
}

func (s *OrderSuite) TestOrder() {
	s.Equal(1, 1)
}

func TestOrderSuite(t *testing.T) {
	suite.Run(t, new(OrderSuite))
}
`

// shadowedImportSrc names a local variable like the testify packages, and a
// plain helper still uses the dot-imported assert.
const shadowedImportSrc = `package sample

import (
	"testing"

	. "github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

import "strings"

type item struct{ name string }

type ShadowSuite struct {
	suite.Suite
}

func (s *ShadowSuite) TestNames() {
	require.NotEmpty(s.T(), "x")
	for _, suite := range []item{{name: "x"}} {
		require := item{name: strings.ToUpper(suite.name)}
		s.Require().Equal("X", require.name)
	}
}

func plain(t *testing.T) {
	Equal(t, 1, 1)
}

func TestShadowSuite(t *testing.T) {
	suite.Run(t, new(ShadowSuite))
}
`

func (s *MigrateTestSuite) TestImportResolution(t *gotest.T) {
	out := s.migrateSource(t, shadowedImportSrc)

	t.When("a local variable is named like a testify package", func(w *gotest.T) {
		w.It("drops the imports only that variable spelled", func(it *gotest.T) {
			gotest.NotContains(it, out, `"github.com/stretchr/testify/suite"`)
			gotest.NotContains(it, out, `"github.com/stretchr/testify/require"`)
		})
	})

	t.When("a dot-imported assertion is still called", func(w *gotest.T) {
		w.It("keeps the dot import", func(it *gotest.T) {
			gotest.Contains(it, out, `. "github.com/stretchr/testify/assert"`)
			gotest.Contains(it, out, "\tEqual(t, 1, 1)")
		})
	})

	t.When("the file has several import blocks", func(w *gotest.T) {
		w.It("adds the gotest import once", func(it *gotest.T) {
			gotest.Equal(it, 1, strings.Count(out, `"github.com/mvrahden/go-test/pkg/gotest"`))
		})
	})

	t.It("produces source that parses", func(it *gotest.T) {
		_, err := parser.ParseFile(token.NewFileSet(), "sample_test.go", out, parser.ParseComments)
		gotest.NoError(it, err)
	})
}

// sharedImportSrc keeps a plain *testing.T helper that still calls assert.
const sharedImportSrc = `package sample

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type SharedSuite struct {
	suite.Suite
}

func (s *SharedSuite) TestOK() {
	require.Equal(s.T(), 1, 1)
}

func plain(t *testing.T) {
	assert.Equal(t, 1, 1)
}

func TestSharedSuite(t *testing.T) {
	suite.Run(t, new(SharedSuite))
}
`

func (s *MigrateTestSuite) TestImports(t *gotest.T) {
	t.When("a non-suite function still uses a testify package", func(w *gotest.T) {
		out := s.migrateSource(w, sharedImportSrc)

		w.It("keeps that import and the testing import it needs", func(it *gotest.T) {
			gotest.Contains(it, out, `"github.com/stretchr/testify/assert"`)
			gotest.Contains(it, out, `"testing"`)
		})

		w.It("drops the imports nothing uses any more", func(it *gotest.T) {
			gotest.NotContains(it, out, `"github.com/stretchr/testify/require"`)
			gotest.NotContains(it, out, `"github.com/stretchr/testify/suite"`)
		})

		w.It("leaves the helper's call alone", func(it *gotest.T) {
			gotest.Contains(it, out, "assert.Equal(t, 1, 1)")
			gotest.Contains(it, out, "gotest.Equal(t, 1, 1)")
		})
	})
}

// helperAssertSrc asserts from a helper without t and from one that takes
// a *testing.T.
const helperAssertSrc = `package sample

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type HelperSuite struct {
	suite.Suite
}

func (s *HelperSuite) helper() {
	s.T().Log("x")
	s.Require().True(true)
}

func (s *HelperSuite) check(t *testing.T) {
	s.Require().Equal(1, 1)
	s.T().Log("y")
}

func (s *HelperSuite) TestA() {
	s.helper()
	s.check(s.T())
}

func TestHelperSuite(t *testing.T) {
	suite.Run(t, new(HelperSuite))
}
`

func (s *MigrateTestSuite) TestHelperAssertions(t *gotest.T) {
	out := s.migrateSource(t, helperAssertSrc)

	t.When("a helper has no t in scope", func(w *gotest.T) {
		w.It("leaves its assertion unrewritten and marks it", func(it *gotest.T) {
			gotest.Contains(it, out,
				"\t// TODO(gotest-migrate): assertion True outside a migrated method — no t is in scope here; pass t in and rewrite as gotest.True(t, ...)\n\ts.Require().True(true)")
			gotest.NotContains(it, out, "gotest.True(t, true)")
		})
	})

	t.When("a helper takes t *testing.T", func(w *gotest.T) {
		w.It("rewrites its assertion against that t", func(it *gotest.T) {
			gotest.Contains(it, out, "func (s *HelperTestSuite) check(t *testing.T) {\n\tgotest.Equal(t, 1, 1)")
		})

		w.It("marks its s.T(), which has no t.T() to become", func(it *gotest.T) {
			gotest.Contains(it, out,
				"\t// TODO(gotest-migrate): s.T() where t is a *testing.T — use t\n\ts.T().Log(\"y\")")
		})
	})

	t.It("still rewrites s.T() in the test method", func(it *gotest.T) {
		gotest.Contains(it, out, "s.check(t.T())")
	})
}

// typeRefSrc refers to its suite outside the suite's own methods.
const typeRefSrc = `package sample

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type RefSuite struct {
	suite.Suite
}

func (s *RefSuite) TestOK() {
	check(s)
}

func check(s *RefSuite) *RefSuite {
	s.Require().True(true)
	return &RefSuite{}
}

func TestRefSuite(t *testing.T) {
	suite.Run(t, new(RefSuite))
}
`

// siblingRefSrc refers to typeRefSrc's suite from another file of the package.
const siblingRefSrc = `package sample

var _ = new(RefSuite)

func other(s *RefSuite) {}
`

func (s *MigrateTestSuite) TestTypeReferences(t *gotest.T) {
	t.When("a free function in the same file takes the suite", func(w *gotest.T) {
		out := s.migrateSource(w, typeRefSrc)

		w.It("renames every reference to the suite type", func(it *gotest.T) {
			gotest.Contains(it, out, "func check(s *RefTestSuite) *RefTestSuite {")
			gotest.Contains(it, out, "return &RefTestSuite{}")
			gotest.NotContains(it, out, "RefSuite")
		})

		w.It("marks the helper's assertion, which has no t", func(it *gotest.T) {
			gotest.Contains(it, out,
				"\t// TODO(gotest-migrate): assertion True outside a migrated method — no t is in scope here; pass t in and rewrite as gotest.True(t, ...)\n\ts.Require().True(true)")
		})
	})

	t.When("another file of the package refers to the suite", func(w *gotest.T) {
		run := migrateModule(w, s.Mod, migrate.Options{}, map[string]string{
			"sample/ref_test.go":     typeRefSrc,
			"sample/sibling_test.go": siblingRefSrc,
		}, "sample")
		sibling := run.files["sample/sibling_test.go"]

		w.It("renames the references there too", func(it *gotest.T) {
			gotest.Contains(it, sibling, "var _ = new(RefTestSuite)")
			gotest.Contains(it, sibling, "func other(s *RefTestSuite) {}")
		})

		w.It("reports the file as updated, not as a suite", func(it *gotest.T) {
			gotest.Len(it, run.results, 2)
			var refs []string
			for i := range run.results {
				r := &run.results[i]
				if r.RefsOnly {
					refs = append(refs, r.File)
				}
			}
			gotest.Equal(it, []string{filepath.Join(run.dir, "sample", "sibling_test.go")}, refs)
		})
	})
}

func (s *MigrateTestSuite) TestScopedRewrites(t *gotest.T) {
	t.When("s.T() appears in a method that gained t and in a helper that did not", func(w *gotest.T) {
		out := s.migrateSource(w, scopedSrc)

		w.It("rewrites s.T() to t.T() only where t is in scope", func(it *gotest.T) {
			gotest.Contains(it, out, "logTo(t.T())")
			gotest.Contains(it, out, "s.T().Helper()")
		})

		w.It("marks the s.T() that has no t", func(it *gotest.T) {
			gotest.Contains(it, out,
				"// TODO(gotest-migrate): s.T() outside a migrated method — no t is in scope here\n\ts.T().Helper()")
		})

		w.It("turns s.Run into t.It, whose own t the assertions inside name", func(it *gotest.T) {
			gotest.Contains(it, out, "\tt.It(\"sub\", func(t *gotest.T) {\n\t\tgotest.Equal(t, 1, 1)\n\t})")
		})
	})

	t.When("a suite embeds another suite instead of suite.Suite", func(w *gotest.T) {
		run := s.migrateFile(w, indirectSrc)
		out := run.files["sample/sample_test.go"]

		w.It("refuses the file and says why in the result", func(it *gotest.T) {
			gotest.Len(it, run.results, 1)
			gotest.Contains(it, run.results[0].Refusal, "OrderSuite embeds BaseSuite")
			gotest.Equal(it, 1, run.results[0].Markers)
		})

		w.It("leaves the source unmigrated apart from the marker", func(it *gotest.T) {
			gotest.Contains(it, out,
				"// TODO(gotest-migrate): OrderSuite embeds BaseSuite, not suite.Suite — migrate this file by hand\ntype OrderSuite struct {")
			gotest.Contains(it, out, "suite.Run(t, new(OrderSuite))")
			gotest.NotContains(it, out, "BaseTestSuite")
		})
	})

	t.When("a file was migrated before and kept its markers", func(w *gotest.T) {
		run := s.migrateFile(w, todoInputSrc)
		results := rerun(w, run)

		w.It("reports the markers still there, with no suite", func(it *gotest.T) {
			gotest.Len(it, results, 1)
			gotest.Equal(it, run.markers(), results[0].Markers)
			gotest.NotZero(it, results[0].Markers)
			gotest.Empty(it, results[0].OldName)
		})
	})

	t.When("a refused file is run again", func(w *gotest.T) {
		run := s.migrateFile(w, indirectSrc)
		results := rerun(w, run)

		w.It("still counts the marker", func(it *gotest.T) {
			gotest.Len(it, results, 1)
			gotest.Equal(it, 1, results[0].Markers)
		})
	})
}
