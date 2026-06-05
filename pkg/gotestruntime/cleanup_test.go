package gotestruntime

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

	t.Run("skip with subtest path uses first segment", func(t *testing.T) {
		gotest.Equal(t, countMatching(names, "", "TestBatchTestSuite/TestDispatch"), 2)
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
