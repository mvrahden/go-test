package canary_test //nolint:fail-guard

import (
	"regexp"
	"slices"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// CanaryDiscoveryTestSuite runs the shapes discovery reads or refuses.
// Sequential: it builds the CLI once and each method spawns full pipeline runs.
type CanaryDiscoveryTestSuite struct{ canary }

func (s *CanaryDiscoveryTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Timeout: 90 * time.Second, SetupTimeout: 30 * time.Second}
}

func (s *CanaryDiscoveryTestSuite) BeforeAll(t *gotest.T) { s.open(t) }

func (s *CanaryDiscoveryTestSuite) AfterAll(t *gotest.T) {}

// shapeRow is one shapes package: the exit it ends with, a test that must
// pass in it and a line its output must show.
type shapeRow struct {
	name, shows, passes string
	exit                int
}

// checkShapes runs each row's shapes package and reports every row that
// ended otherwise.
func (s *canary) checkShapes(t *gotest.T, rows []shapeRow) {
	for _, row := range rows {
		r := s.run(t, "shapes/"+row.name, t.TempDir())
		if r.code != row.exit {
			t.Errorf("shapes %s: exit code %d, want %d\n%s", row.name, r.code, row.exit, r.stderr)
			continue
		}
		if row.passes != "" && !slices.Contains(r.rows, "shapes/"+row.name+" pass "+row.passes) {
			t.Errorf("shapes %s: %s did not pass: %v\n%s", row.name, row.passes, r.rows, r.stderr)
		}
		if row.shows != "" && !regexp.MustCompile(row.shows).MatchString(r.stream+r.stderr) {
			t.Errorf("shapes %s: no line matches %q\n%s", row.name, row.shows, r.stderr)
		}
	}
}

// A suite or fixture is one type declaration of its own, composed through
// named fields; any other shape is refused with exit 2 at the line written.
func (s *CanaryDiscoveryTestSuite) TestDiscoveryShapesAreRejectedWhereWritten(t *gotest.T) {
	s.checkShapes(t, []shapeRow{
		{name: "plain", passes: "TestPlainTestSuite/TestOpened"},
		{name: "plain", passes: "TestEmbedsPlainTestSuite/TestBumps"},
		{name: "grouped", exit: 2, shows: `grouped_suite_test\.go:7:2: suite GroupedTestSuite is declared inside a type block; give it its own type declaration`},
		{name: "groupedfixture", exit: 2, shows: `groupedfixture_suite_test\.go:11:2: fixture GroupedFixture is declared inside a type block; give it its own type declaration`},
		{name: "embedsuite", exit: 2, shows: `embedsuite_suite_test\.go:12:2: suite ChildTestSuite embeds suite BaseTestSuite; suites are never embedded — share behavior through a generic contract suite`},
		{name: "embedfixture", exit: 2, shows: `embedfixture_suite_test\.go:16:3: suite ChildTestSuite embeds fixture StoreFixture; fixtures compose through a named pointer field`},
		{name: "embedhooks", exit: 2, shows: `embedhooks_suite_test\.go:12:2: suite HookedTestSuite embeds base, whose method TestFromBase gotest would read; keep helpers in a named field or in functions`},
		{name: "nested", exit: 2, shows: `nested_suite_test\.go:12:2: suite OuterTestSuite has a field of suite type InnerTestSuite; suites are never nested — share behavior through a generic contract suite`},
		{name: "fixtureembeds", exit: 2, shows: `fixtureembeds_suite_test\.go:16:3: fixture CacheFixture embeds fixture DBFixture; fixtures compose through a named pointer field`},
	})
}

// An alias names the suite it stands for, also across packages; a defined
// type, a case with results, a shared fixture in a test file and a mis-signed
// hook from another package are rejected where they are written.
func (s *CanaryDiscoveryTestSuite) TestAliasesAndCrossPackageShapes(t *gotest.T) {
	s.checkShapes(t, []shapeRow{
		{name: "aliasrecv", passes: "TestRealTestSuite/TestViaAlias"},
		{name: "aliasalias", passes: "TestIntTestSuite/TestZero"},
		{name: "aliasalias", passes: "TestIntAgainTestSuite/TestZero"},
		{name: "xpkgcontract", passes: "TestIntIndexTestSuite/TestZeroValue"},
		{name: "sigdefined", exit: 2, shows: `sigdefined_suite_test\.go:10:6: suite CopyTestSuite is defined over suite ATestSuite and carries none of its methods; declare an alias \(= ATestSuite\) or a struct of its own`},
		{name: "sigresult", exit: 2, shows: `sigresult_suite_test\.go:9:22: TestBad on XTestSuite returns a value; a Test method returns nothing`},
		{name: "sharedintest", exit: 2, shows: `sharedintest_suite_test\.go:10:6: shared fixture PGSharedFixture is declared in a _test\.go file; the setup program cannot import it, move it to a non-test file`},
		{name: "xpkgfixture", exit: 2, shows: `fix\.go:11:\d+: package fixture "BadFixture": BeforeEach must have signature \(ctx context\.Context\) error`},
	})
}
