// Ring 0: raw checks only (see ring0_suite_test.go).
package gotestruntime_test //nolint:fail-guard

import (
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/pkg/gotestruntime"
)

// CountMatchingTestSuite covers the -run/-skip prediction that feeds the
// fixture-teardown countdown; every ambiguity must resolve high, never low.
type CountMatchingTestSuite struct{}

func (s *CountMatchingTestSuite) TestCountMatching(t *gotest.T) {
	names := []string{"TestQueryTestSuite", "TestBatchTestSuite", "TestPricingTestSuite"}

	t.It("no flags returns all", func(t *gotest.T) {
		mustEqual(t, gotestruntime.ExportCountMatching(names, "", ""), 3)
	})

	t.It("run exact match", func(t *gotest.T) {
		mustEqual(t, gotestruntime.ExportCountMatching(names, "TestQueryTestSuite", ""), 1)
	})

	t.It("run regex matches all", func(t *gotest.T) {
		mustEqual(t, gotestruntime.ExportCountMatching(names, "Test.*Suite", ""), 3)
	})

	t.It("run regex matches subset", func(t *gotest.T) {
		mustEqual(t, gotestruntime.ExportCountMatching(names, "Test(Query|Batch)", ""), 2)
	})

	t.It("run with subtest path uses first segment", func(t *gotest.T) {
		mustEqual(t, gotestruntime.ExportCountMatching(names, "TestQueryTestSuite/TestInsert", ""), 1)
	})

	t.It("skip one", func(t *gotest.T) {
		mustEqual(t, gotestruntime.ExportCountMatching(names, "", "TestBatchTestSuite"), 2)
	})

	t.It("skip regex", func(t *gotest.T) {
		mustEqual(t, gotestruntime.ExportCountMatching(names, "", "Test(Batch|Pricing)"), 1)
	})

	t.It("run and skip combined", func(t *gotest.T) {
		mustEqual(t, gotestruntime.ExportCountMatching(names, "Test.*Suite", "TestPricingTestSuite"), 2)
	})

	t.It("run no match falls back to all", func(t *gotest.T) {
		mustEqual(t, gotestruntime.ExportCountMatching(names, "TestNonexistent", ""), 3)
	})

	t.It("skip all falls back to all", func(t *gotest.T) {
		mustEqual(t, gotestruntime.ExportCountMatching(names, "", "Test.*"), 3)
	})

	t.It("invalid run regex falls back to all", func(t *gotest.T) {
		mustEqual(t, gotestruntime.ExportCountMatching(names, "[invalid", ""), 3)
	})

	t.It("invalid skip regex ignored", func(t *gotest.T) {
		mustEqual(t, gotestruntime.ExportCountMatching(names, "", "[invalid"), 3)
	})

	t.It("skip with subtest path is ignored", func(t *gotest.T) {
		// go test still RUNS TestBatchTestSuite — only its TestDispatch subtest
		// is skipped — so its execution still decrements the countdown. The old
		// first-segment cut excluded it and tore the fixture DAG down while a
		// fixture-bound test was still to run.
		mustEqual(t, gotestruntime.ExportCountMatching(names, "", "TestBatchTestSuite/TestDispatch"), 3)
	})

	t.It("run with slash inside a character class", func(t *gotest.T) {
		// strings.Cut sheared `Test[Q/B]...` at the class-interior slash into
		// an uncompilable fragment, silently disabling the filter. The
		// bracket-aware split keeps the whole segment.
		mustEqual(t, gotestruntime.ExportCountMatching(names, "Test[QB/]", ""), 2,
			"a class-interior slash must not shear the segment into an uncompilable half that disables the filter")
		mustEqual(t, gotestruntime.ExportCountMatching(names, "Test[QB][a-z]*TestSuite/TestInsert", ""), 2,
			"the depth-0 slash after the classes is still the segment boundary")
	})

	t.It("count errs high, never low", func(t *gotest.T) {
		// The property behind every fallback above: teardown too late is a
		// deferred release, teardown too early corrupts a still-running test.
		mustEqual(t, gotestruntime.ExportCountMatching(names, "([", "(["), 3)
	})

	t.It("single name list", func(t *gotest.T) {
		mustEqual(t, gotestruntime.ExportCountMatching([]string{"TestOnly"}, "", ""), 1)
		mustEqual(t, gotestruntime.ExportCountMatching([]string{"TestOnly"}, "TestOnly", ""), 1)
		mustEqual(t, gotestruntime.ExportCountMatching([]string{"TestOnly"}, "TestOther", ""), 1)
	})

	t.It("empty name list", func(t *gotest.T) {
		mustEqual(t, gotestruntime.ExportCountMatching([]string{}, "", ""), 0)
	})
}
