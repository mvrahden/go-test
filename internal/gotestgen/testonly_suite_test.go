package gotestgen_test

import (
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/pkg/gotest"
)

type TestOnlyTestSuite struct{}

func (s *TestOnlyTestSuite) TestIsTestOnly(t *gotest.T) {
	t.When("example packages", func(w *gotest.T) {
		w.It("reports IsTestOnly correctly for each", func(it *gotest.T) {
			examplesDir := filepath.Join("..", "..", "examples")
			absExamples, err := filepath.Abs(examplesDir)
			gotest.NoError(it, err)

			origDir, err := os.Getwd()
			gotest.NoError(it, err)

			err = os.Chdir(absExamples)
			gotest.NoError(it, err)
			defer os.Chdir(origDir)

			tests := []struct {
				pattern  string
				expected bool
			}{
				{"./cart", false},
				{"./auth", false},
				{"./search", false},
			}

			for _, tc := range tests {
				results, _, err := gotestgen.LoadPackagesForDiscovery([]string{tc.pattern}, nil)
				gotest.NoError(it, err)
				gotest.NotEmpty(it, results, "no packages found for %s", tc.pattern)

				got := results[0].IsTestOnly()
				gotest.Equal(it, tc.expected, got, "IsTestOnly() for %s", tc.pattern)
			}
		})
	})
}
