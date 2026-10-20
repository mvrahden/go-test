package canary_test //nolint:fail-guard

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
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

// checkShapes runs each row's shapes package, reports every row that ended
// otherwise and returns each package's last run.
func (s *canary) checkShapes(t *gotest.T, rows []shapeRow) map[string]result {
	runs := map[string]result{}
	for _, row := range rows {
		r := s.run(t, "shapes/"+row.name, t.TempDir())
		runs[row.name] = r
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
	return runs
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

// An alias names the suite it stands for, also across packages and from the
// external test package; a defined type, a case with results, a shared
// fixture in a test file and a mis-signed hook from another package are
// rejected where they are written.
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
		{name: "pxgenericalias", passes: "TestIntContractTestSuite/TestRuns"},
		{name: "pxgenericalias", passes: "TestIntLocalTestSuite/TestLocal"},
		{name: "pxgenericalias", passes: "TestIntIndexTestSuite/TestZeroValue"},
	})
}

// A fixture is bound once, through a pointer field, and only a bound one is
// checked; a method is a case only by the name go test would run, and a
// suite with no case is a helper nothing is generated or reported for.
func (s *CanaryDiscoveryTestSuite) TestFieldNameAndCaselessShapes(t *gotest.T) {
	runs := s.checkShapes(t, []shapeRow{
		{name: "fieldvalue", exit: 2, shows: `fieldvalue_suite_test\.go:16:2: suite ValueTestSuite has fixture field Store of value type StoreFixture; a fixture is bound through a pointer field \(\*StoreFixture\)`},
		{name: "fieldalias", exit: 2, shows: `fieldalias_suite_test\.go:19:2: suite AliasTestSuite has fixture field Store of value type StoreFixture; a fixture is bound through a pointer field \(\*StoreFixture\)`},
		{name: "fieldaliasptr", passes: "TestAliasPtrTestSuite/TestOpened"},
		{name: "fielddup", exit: 2, shows: `fielddup_suite_test\.go:16:5: suite DupTestSuite declares two fields of the same fixture type StoreFixture \(A and B\); keep one`},
		{name: "fielddupparent", exit: 2, shows: `fielddupparent_suite_test\.go:17:2: fixture CacheFixture declares two fields of the same fixture type StoreFixture \(A and B\); keep one`},
		{name: "fieldunreferenced", passes: "TestBoundTestSuite/TestOpened"},
		{name: "nameistest", passes: "TestNameTestSuite/TestRuns"},
		{name: "caseless", passes: "TestRealTestSuite/TestNames"},
	})
	for _, helper := range []string{"Testify", "Benchmarked", "Fuzzy"} {
		if strings.Contains(runs["nameistest"].stream, `"Test":"TestNameTestSuite/`+helper+`"`) {
			t.Errorf("shapes nameistest: %s ran as a case: %v", helper, runs["nameistest"].rows)
		}
	}
	if r := runs["caseless"]; strings.Contains(r.stream, "HelperTestSuite") || strings.Contains(r.stderr, "HelperTestSuite") {
		t.Errorf("shapes caseless: the run reported HelperTestSuite: %v\n%s", r.rows, r.stderr)
	}
	code, stdout, stderr := s.invoke(t, t.TempDir(), "discover", "./testdata/shapes/caseless/")
	switch {
	case code != 0:
		t.Errorf("shapes caseless: discover exit code %d, want 0\n%s", code, stderr)
	case !bytes.Contains(stdout, []byte("RealTestSuite")):
		t.Errorf("shapes caseless: discover does not list RealTestSuite:\n%s", stdout)
	case bytes.Contains(stdout, []byte("HelperTestSuite")):
		t.Errorf("shapes caseless: discover lists HelperTestSuite:\n%s", stdout)
	}
}

// The generated harness compiles beside whatever names the package declares,
// a stale generated file on disk is never read, and a second generate after
// a rename leaves nothing of the old suite.
func (s *CanaryDiscoveryTestSuite) TestGeneratedFilesStayValid(t *gotest.T) {
	s.checkShapes(t, []shapeRow{
		{name: "gencontext", passes: "TestShadowsTestSuite/TestRuns"},
		{name: "genpgpg", passes: "TestBothTestSuite/TestBoth"},
		{name: "stalerename", passes: "TestNewTestSuite/TestRuns"},
	})
	code, stdout, stderr := s.invoke(t, t.TempDir(), "discover", "./testdata/shapes/stalerename/")
	switch {
	case code != 0:
		t.Errorf("shapes stalerename: discover exit code %d, want 0\n%s", code, stderr)
	case !bytes.Contains(stdout, []byte(`"name":"NewTestSuite"`)) || bytes.Contains(stdout, []byte("OldTestSuite")):
		t.Errorf("shapes stalerename: discover does not list NewTestSuite alone:\n%s", stdout)
	}

	root, err := filepath.Abs("../..")
	if err != nil {
		fatalf(t, "abs: %v", err)
	}
	dir := t.TempDir()
	pkg, err := testkit.StagePackage(root, filepath.Join("testdata", "shapes", "genrename"), dir, "example.com/genrename")
	if err != nil {
		fatalf(t, "shapes genrename: stage: %v", err)
	}
	generate := func() bool {
		code, stdout, stderr := s.invokeIn(t, dir, t.TempDir(), "generate", "./...")
		if code != 0 {
			t.Errorf("shapes genrename: generate exit code %d, want 0\n%s%s", code, stdout, stderr)
		}
		return code == 0
	}
	if !generate() {
		return
	}
	if err := os.Remove(filepath.Join(pkg, "ext_suite_test.go")); err != nil {
		fatalf(t, "shapes genrename: %v", err)
	}
	suite := filepath.Join(pkg, "genrename_suite_test.go")
	src, err := os.ReadFile(suite) //nolint:gosec // G304: a file in the test's own temp dir
	if err != nil {
		fatalf(t, "shapes genrename: %v", err)
	}
	if err := os.WriteFile(suite, bytes.ReplaceAll(src, []byte("OldName"), []byte("NewName")), 0o600); err != nil {
		fatalf(t, "shapes genrename: %v", err)
	}
	if !generate() {
		return
	}
	harness, err := os.ReadFile(filepath.Join(pkg, "gotest_psuite_test.go")) //nolint:gosec // G304: a file in the test's own temp dir
	switch {
	case err != nil:
		t.Errorf("shapes genrename: %v", err)
	case bytes.Contains(harness, []byte("OldName")) || !bytes.Contains(harness, []byte("NewNameTestSuite")):
		t.Errorf("shapes genrename: the harness does not name NewNameTestSuite alone:\n%s", harness)
	}
	if _, err := os.Stat(filepath.Join(pkg, "gotest_pxsuite_test.go")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("shapes genrename: the external harness of a variant without suites is still there: %v", err)
	}
	code, stdout, stderr = s.invokeIn(t, dir, t.TempDir(), "-json", "./genrename/")
	passed := false
	for _, ev := range testkit.ParseEvents(stdout) {
		passed = passed || (ev.Action == "pass" && ev.Test == "TestNewNameTestSuite/TestRuns")
	}
	if code != 0 || !passed {
		t.Errorf("shapes genrename: the run after the rename exits %d, TestNewNameTestSuite/TestRuns passed %v\n%s", code, passed, stderr)
	}
}

// Focus is decided once for the whole run: a focused suite or method in one
// package or test package variant skips the unfocused suites of every other,
// and a suite skipped that way starts none of its fixtures.
func (s *CanaryDiscoveryTestSuite) TestFocusIsRunWide(t *gotest.T) {
	verdicts := func(stdout []byte) map[string]string {
		out := map[string]string{}
		for _, ev := range testkit.ParseEvents(stdout) {
			if ev.Action == "pass" || ev.Action == "fail" || ev.Action == "skip" {
				out[ev.Test] = ev.Action
			}
		}
		return out
	}

	dir := t.TempDir()
	code, stdout, stderr := s.invoke(t, dir, "-json", "./testdata/shapes/focusacross/...")
	got := verdicts(stdout)
	if code != 0 {
		t.Errorf("shapes focusacross: exit code %d, want 0\n%s", code, stderr)
	}
	if got["TestF_OnlyTestSuite/TestRuns"] != "pass" {
		t.Errorf("shapes focusacross: TestF_OnlyTestSuite/TestRuns did not pass: %v\n%s", got, stderr)
	}
	if got["TestOtherTestSuite"] != "skip" {
		t.Errorf("shapes focusacross: TestOtherTestSuite is %q, want skip: %v", got["TestOtherTestSuite"], got)
	}
	if v, ok := got["TestOtherTestSuite/TestRuns"]; ok {
		t.Errorf("shapes focusacross: TestOtherTestSuite/TestRuns has verdict %q, want none", v)
	}
	markers, err := testkit.ReadMarkers(filepath.Join(dir, testkit.MarkersFile))
	if err != nil {
		fatalf(t, "shapes focusacross: %v", err)
	}
	if !slices.Contains(markers, "focused ran") {
		t.Errorf("shapes focusacross: the focused suite left no marker: %v", markers)
	}
	for _, m := range []string{"fixture up", "shared up"} {
		if slices.Contains(markers, m) {
			t.Errorf("shapes focusacross: the focus-skipped suite started a fixture: %q in %v", m, markers)
		}
	}

	code, stdout, stderr = s.invoke(t, t.TempDir(), "-json", "./testdata/shapes/focusvariants/")
	got = verdicts(stdout)
	if code != 0 {
		t.Errorf("shapes focusvariants: exit code %d, want 0\n%s", code, stderr)
	}
	if got["TestF_InTestSuite/TestRuns"] != "pass" {
		t.Errorf("shapes focusvariants: TestF_InTestSuite/TestRuns did not pass: %v\n%s", got, stderr)
	}
	if got["TestExtTestSuite"] != "skip" {
		t.Errorf("shapes focusvariants: TestExtTestSuite is %q, want skip: %v", got["TestExtTestSuite"], got)
	}

	code, stdout, stderr = s.invoke(t, t.TempDir(), "-json", "./testdata/shapes/focusmethod/...")
	got = verdicts(stdout)
	if code != 0 {
		t.Errorf("shapes focusmethod: exit code %d, want 0\n%s", code, stderr)
	}
	if got["TestMethodTestSuite/F_TestFocused"] != "pass" {
		t.Errorf("shapes focusmethod: TestMethodTestSuite/F_TestFocused did not pass: %v\n%s", got, stderr)
	}
	if v, ok := got["TestMethodTestSuite/TestPlain"]; ok {
		t.Errorf("shapes focusmethod: TestMethodTestSuite/TestPlain has verdict %q, want none", v)
	}
	if got["TestOtherTestSuite"] != "skip" {
		t.Errorf("shapes focusmethod: TestOtherTestSuite is %q, want skip: %v", got["TestOtherTestSuite"], got)
	}
}
