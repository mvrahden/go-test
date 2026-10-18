package conventions_test

import (
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/internal/speccatalog"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CatalogTestSuite holds docs/design/catalog.{md,json} to what the
// conformance suites say: a fresh render must match the committed files, and
// no behavior may be one the static reading could not enumerate.
// Sequential: both methods share one render, which builds and runs the CLI.
type CatalogTestSuite struct {
	root   string
	md, js []byte
}

func (s *CatalogTestSuite) BeforeAll(t *gotest.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	gotest.NoError(t, err)
	s.root = root
	s.md, s.js, err = speccatalog.Render(t.Context(), root)
	gotest.NoError(t, err)
}

func (s *CatalogTestSuite) AfterAll(t *gotest.T) {}

func (s *CatalogTestSuite) TestCatalogIsCurrent(t *gotest.T) {
	t.When("the catalog is rendered from source", func(w *gotest.T) {
		w.It("matches the committed markdown", func(it *gotest.T) {
			committed, err := os.ReadFile(filepath.Join(s.root, speccatalog.MarkdownPath))
			gotest.NoError(it, err, "run make catalog")
			gotest.Equal(it, string(s.md), string(committed), "run make catalog")
		})
		w.It("matches the committed json", func(it *gotest.T) {
			committed, err := os.ReadFile(filepath.Join(s.root, speccatalog.JSONPath))
			gotest.NoError(it, err, "run make catalog")
			gotest.Equal(it, string(s.js), string(committed), "run make catalog")
		})
	})
}

func (s *CatalogTestSuite) TestCatalogIsComplete(t *gotest.T) {
	t.When("the static reading is inspected", func(w *gotest.T) {
		w.It("marks no behavior incomplete", func(it *gotest.T) {
			gotest.NotContains(it, string(s.md), "INCOMPLETE")
			gotest.NotContains(it, string(s.md), "_Incomplete")
			gotest.NotContains(it, string(s.js), `"incomplete": true`)
		})
	})
}
