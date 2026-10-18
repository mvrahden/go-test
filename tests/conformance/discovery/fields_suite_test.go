package discovery_test

import (
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// DiscoveryFieldsTestSuite: a suite binds each fixture once, through a pointer
// field; only the types it binds are fixtures to it, and a method is a case
// only by the name go test would run.
type DiscoveryFieldsTestSuite struct{}

func (s *DiscoveryFieldsTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second}
}

func (s *DiscoveryFieldsTestSuite) TestFieldsAndNames(t *gotest.T) {
	t.When("a suite names its fixtures and methods", func(w *gotest.T) {
		w.It("a fixture field by value is rejected at the field and told to take a pointer", func(it *gotest.T) {
			checkShape(it, shape{pkg: "fieldvalue", exit: 2,
				shows: `fieldvalue_suite_test\.go:16:2: suite ValueTestSuite has fixture field Store of value type StoreFixture; a fixture is bound through a pointer field \(\*StoreFixture\)`})
			checkShape(it, shape{pkg: "fieldalias", exit: 2,
				shows: `fieldalias_suite_test\.go:19:2: suite AliasTestSuite has fixture field Store of value type StoreFixture; a fixture is bound through a pointer field \(\*StoreFixture\)`})
		})
		w.It("a fixture field typed by an alias of the fixture pointer binds the fixture", func(it *gotest.T) {
			checkShape(it, shape{pkg: "fieldaliasptr", verdict: "TestAliasPtrTestSuite/TestOpened"})
		})
		w.It("two fields of the same fixture type are rejected at the second field", func(it *gotest.T) {
			checkShape(it, shape{pkg: "fielddup", exit: 2,
				shows: `fielddup_suite_test\.go:16:5: suite DupTestSuite declares two fields of the same fixture type StoreFixture \(A and B\); keep one`})
			checkShape(it, shape{pkg: "fielddupparent", exit: 2,
				shows: `fielddupparent_suite_test\.go:17:2: fixture CacheFixture declares two fields of the same fixture type StoreFixture \(A and B\); keep one`})
		})
		w.It("a fixture-named type no suite reaches cannot fail the package", func(it *gotest.T) {
			checkShape(it, shape{pkg: "fieldunreferenced", verdict: "TestBoundTestSuite/TestOpened"})
		})
		w.It("a method whose name continues Test with a lowercase letter is not a case, as under go test", func(it *gotest.T) {
			run, err := testkit.RunPackage(it.Context(), it.TempDir(), "../testdata/shapes/nameistest/")
			gotest.NoError(it, err)
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Equal(it, "pass", run.Verdict("TestNameTestSuite/TestRuns"), "stdout: %s", run.Stdout)
			var listed []string
			for _, ev := range run.Events {
				switch ev.Test {
				case "TestNameTestSuite/Testify", "TestNameTestSuite/Benchmarked", "TestNameTestSuite/Fuzzy":
					listed = append(listed, ev.Test)
				}
			}
			gotest.Empty(it, listed)
		})
	})
}
