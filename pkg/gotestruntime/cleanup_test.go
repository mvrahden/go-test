package gotestruntime //nolint:stdlib-test

import (
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

func TestCountMatching(t *testing.T) {
	names := []string{"TestQueryTestSuite", "TestBatchTestSuite", "TestPricingTestSuite"}

	t.Run("no flags returns all", func(t *testing.T) {
		gotest.Equal(t, countMatching(names, "", ""), 3)
	})

	t.Run("run exact match", func(t *testing.T) {
		gotest.Equal(t, countMatching(names, "TestQueryTestSuite", ""), 1)
	})

	t.Run("run regex matches all", func(t *testing.T) {
		gotest.Equal(t, countMatching(names, "Test.*Suite", ""), 3)
	})

	t.Run("run regex matches subset", func(t *testing.T) {
		gotest.Equal(t, countMatching(names, "Test(Query|Batch)", ""), 2)
	})

	t.Run("run with subtest path uses first segment", func(t *testing.T) {
		gotest.Equal(t, countMatching(names, "TestQueryTestSuite/TestInsert", ""), 1)
	})

	t.Run("skip one", func(t *testing.T) {
		gotest.Equal(t, countMatching(names, "", "TestBatchTestSuite"), 2)
	})

	t.Run("skip regex", func(t *testing.T) {
		gotest.Equal(t, countMatching(names, "", "Test(Batch|Pricing)"), 1)
	})

	t.Run("run and skip combined", func(t *testing.T) {
		gotest.Equal(t, countMatching(names, "Test.*Suite", "TestPricingTestSuite"), 2)
	})

	t.Run("run no match falls back to all", func(t *testing.T) {
		gotest.Equal(t, countMatching(names, "TestNonexistent", ""), 3)
	})

	t.Run("skip all falls back to all", func(t *testing.T) {
		gotest.Equal(t, countMatching(names, "", "Test.*"), 3)
	})

	t.Run("invalid run regex falls back to all", func(t *testing.T) {
		gotest.Equal(t, countMatching(names, "[invalid", ""), 3)
	})

	t.Run("invalid skip regex ignored", func(t *testing.T) {
		gotest.Equal(t, countMatching(names, "", "[invalid"), 3)
	})

	t.Run("skip with subtest path is ignored", func(t *testing.T) {
		// go test still RUNS TestBatchTestSuite — only its TestDispatch subtest
		// is skipped — so its execution still decrements the countdown. The old
		// first-segment cut excluded it and tore the fixture DAG down while a
		// fixture-bound test was still to run.
		gotest.Equal(t, countMatching(names, "", "TestBatchTestSuite/TestDispatch"), 3)
	})

	t.Run("run with slash inside a character class", func(t *testing.T) {
		// strings.Cut sheared `Test[Q/B]...` at the class-interior slash into
		// an uncompilable fragment, silently disabling the filter. The
		// bracket-aware split keeps the whole segment.
		gotest.Equal(t, countMatching(names, "Test[QB/]", ""), 2,
			"a class-interior slash must not shear the segment into an uncompilable half that disables the filter")
		gotest.Equal(t, countMatching(names, "Test[QB][a-z]*TestSuite/TestInsert", ""), 2,
			"the depth-0 slash after the classes is still the segment boundary")
	})

	t.Run("count errs high, never low", func(t *testing.T) {
		// The property behind every fallback above: teardown too late is a
		// deferred release, teardown too early corrupts a still-running test.
		gotest.Equal(t, countMatching(names, "([", "(["), 3)
	})

	t.Run("single name list", func(t *testing.T) {
		gotest.Equal(t, countMatching([]string{"TestOnly"}, "", ""), 1)
		gotest.Equal(t, countMatching([]string{"TestOnly"}, "TestOnly", ""), 1)
		gotest.Equal(t, countMatching([]string{"TestOnly"}, "TestOther", ""), 1)
	})

	t.Run("empty name list", func(t *testing.T) {
		gotest.Equal(t, countMatching([]string{}, "", ""), 0)
	})
}
