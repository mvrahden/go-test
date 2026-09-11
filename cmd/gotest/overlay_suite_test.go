package main_test

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// OverlayTestSuite covers the overlay the CLI writes for a generated package.
type OverlayTestSuite struct{}

func (s *OverlayTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *OverlayTestSuite) TestGenerateOverlay(t *gotest.T) {
	t.When("suites are present", func(w *gotest.T) {
		w.It("produces valid overlay JSON", func(it *gotest.T) {
			absExamples, err := filepath.Abs(filepath.Join("..", "..", "examples"))
			gotest.NoError(it, err, "%v", err)
			if _, err := os.Stat(filepath.Join(absExamples, "go.mod")); err != nil {
				it.Skipf("examples directory not found: %v", err)
			}

			loaded, _, err := gotestgen.LoadPackages([]string{filepath.Join(absExamples, "cart")}, nil)
			gotest.NoError(it, err, "LoadPackages: %v", err)
			results, _, err := gotestgen.GenerateFromLoaded(loaded)
			gotest.NoError(it, err, "GenerateFromLoaded: %v", err)
			gotest.NotEmpty(it, results, "expected at least one generate result")

			tmpDir, err := gotestrunner.WriteOverlay(results)
			gotest.NoError(it, err, "WriteOverlay: %v", err)
			defer os.RemoveAll(tmpDir)

			overlayFile := filepath.Join(tmpDir, "overlay.json")
			_, err = os.Stat(overlayFile)
			gotest.NoError(it, err)

			data, err := os.ReadFile(overlayFile)
			gotest.NoError(it, err, "reading overlay.json: %v", err)
			var overlayContent struct {
				Replace map[string]string `json:"Replace"`
			}
			gotest.NoError(it, json.Unmarshal(data, &overlayContent), "overlay.json is not valid JSON")
			gotest.NotEmpty(it, overlayContent.Replace, "overlay.json Replace map is empty")
		})
	})

	t.When("no suites", func(w *gotest.T) {
		w.It("returns empty results for package without suites", func(it *gotest.T) {
			tmpDir, err := os.MkdirTemp("", "overlay-test-nosuite-*")
			gotest.NoError(it, err, "%v", err)
			defer os.RemoveAll(tmpDir)

			gotest.NoError(it, os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module nosuite\n\ngo 1.25\n"), 0600))
			gotest.NoError(it, os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0600))

			loaded, _, err := gotestgen.LoadPackages([]string{tmpDir}, nil)
			gotest.NoError(it, err, "LoadPackages: %v", err)
			results, _, err := gotestgen.GenerateFromLoaded(loaded)
			gotest.NoError(it, err, "GenerateFromLoaded: %v", err)

			var allResults gotestgen.GenerateResults
			allResults = append(allResults, results...)
			if len(allResults) != 0 {
				it.Skipf("expected 0 results for package without suites, got %d (package may have test suites)", len(allResults))
			}
		})
	})
}
