package gotestgen_test

import (
	"path/filepath"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// TestOnlyTestSuite tests detection of test-only packages.
type TestOnlyTestSuite struct{}

func (s *TestOnlyTestSuite) TestIsTestOnly(t *gotest.T) {
	t.When("example packages", func(w *gotest.T) {
		w.It("reports IsTestOnly correctly for each", func(it *gotest.T) {
			absExamples, err := filepath.Abs(filepath.Join("..", "..", "examples"))
			gotest.NoError(it, err)

			tests := []struct {
				pattern  string
				expected bool
			}{
				{"cart", false},
				{"auth", false},
				{"search", false},
			}

			for _, tc := range tests {
				results, _, err := gotestgen.LoadPackages([]string{filepath.Join(absExamples, tc.pattern)}, nil)
				gotest.NoError(it, err)
				gotest.NotEmpty(it, results, "no packages found for %s", tc.pattern)

				got := results[0].IsTestOnly()
				gotest.Equal(it, tc.expected, got, "IsTestOnly() for %s", tc.pattern)
			}
		})
	})
}
