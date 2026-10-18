package discovery_test

import (
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// DiscoveryShapesTestSuite: a suite or fixture is one type declaration of its
// own, composed through named fields; every other shape is rejected where it
// is written, with the alternative named.
type DiscoveryShapesTestSuite struct{}

func (s *DiscoveryShapesTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second}
}

type shape struct {
	pkg     string
	exit    int
	shows   string // regexp the run's output must match
	verdict string // a method that passes, for accepted shapes
}

func (s *DiscoveryShapesTestSuite) TestDeclarationsAndFields(t *gotest.T) {
	t.When("a suite or fixture is declared", func(w *gotest.T) {
		w.It("a suite declared alone, binding a fixture through a named pointer field, runs", func(it *gotest.T) {
			checkShape(it, shape{pkg: "plain", verdict: "TestPlainTestSuite/TestOpened"})
		})
		w.It("a suite embedding a plain type runs", func(it *gotest.T) {
			checkShape(it, shape{pkg: "plain", verdict: "TestEmbedsPlainTestSuite/TestBumps"})
		})
		w.It("a suite in a type block is rejected at its declaration and told to stand alone", func(it *gotest.T) {
			checkShape(it, shape{pkg: "grouped", exit: 2,
				shows: `grouped_suite_test\.go:7:2: suite GroupedTestSuite is declared inside a type block; give it its own type declaration`})
		})
		w.It("a fixture in a type block is rejected at its declaration and told to stand alone", func(it *gotest.T) {
			checkShape(it, shape{pkg: "groupedfixture", exit: 2,
				shows: `groupedfixture_suite_test\.go:11:2: fixture GroupedFixture is declared inside a type block; give it its own type declaration`})
		})
		w.It("a suite embedding a suite is rejected at the field and pointed to contract suites", func(it *gotest.T) {
			checkShape(it, shape{pkg: "embedsuite", exit: 2,
				shows: `embedsuite_suite_test\.go:12:2: suite ChildTestSuite embeds suite BaseTestSuite; suites are never embedded — share behavior through a generic contract suite`})
		})
		w.It("a suite embedding a fixture is rejected at the field and pointed to a named pointer field", func(it *gotest.T) {
			checkShape(it, shape{pkg: "embedfixture", exit: 2,
				shows: `embedfixture_suite_test\.go:16:3: suite ChildTestSuite embeds fixture StoreFixture; fixtures compose through a named pointer field`})
		})
		w.It("a suite embedding a type with a method gotest would read is rejected at the field", func(it *gotest.T) {
			checkShape(it, shape{pkg: "embedhooks", exit: 2,
				shows: `embedhooks_suite_test\.go:12:2: suite HookedTestSuite embeds base, whose method TestFromBase gotest would read; keep helpers in a named field or in functions`})
		})
		w.It("a suite holding a suite in a field is rejected at the field: suites are never nested", func(it *gotest.T) {
			checkShape(it, shape{pkg: "nested", exit: 2,
				shows: `nested_suite_test\.go:12:2: suite OuterTestSuite has a field of suite type InnerTestSuite; suites are never nested — share behavior through a generic contract suite`})
		})
		w.It("a fixture embedding a fixture is rejected at the field and pointed to a named pointer field", func(it *gotest.T) {
			checkShape(it, shape{pkg: "fixtureembeds", exit: 2,
				shows: `fixtureembeds_suite_test\.go:16:3: fixture CacheFixture embeds fixture DBFixture; fixtures compose through a named pointer field`})
		})
	})
}

// checkShape runs the shape's package and checks its exit, verdict and output.
func checkShape(t *gotest.T, sh shape) {
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/shapes/"+sh.pkg+"/")
	gotest.NoError(t, err)
	gotest.Equal(t, sh.exit, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
	if sh.verdict != "" {
		gotest.Equal(t, "pass", run.Verdict(sh.verdict), "stdout: %s", run.Stdout)
	}
	if sh.shows != "" {
		gotest.Regexp(t, sh.shows, run.Stdout+run.Stderr)
	}
}
