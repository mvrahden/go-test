package conventions_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mvrahden/go-test/internal/docref"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// ReferenceTestSuite holds the generated sections of the documents to the
// code they are rendered from.
type ReferenceTestSuite struct{}

func (s *ReferenceTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *ReferenceTestSuite) TestTheReferenceIsCurrent(t *gotest.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	gotest.NoError(t, err)
	t.When("the marked sections are rendered from the code", func(w *gotest.T) {
		w.It("match the committed documents", func(it *gotest.T) {
			stale, err := docref.Stale(root)
			gotest.NoError(it, err)
			gotest.Empty(it, stale)
		})
	})
	t.When("the documents are read for markers", func(w *gotest.T) {
		w.It("every document of the list holds one", func(it *gotest.T) {
			for _, doc := range docref.Documents {
				data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(doc))) //nolint:gosec // G304: a document of this repository
				gotest.NoError(it, err)
				marks, err := docref.Sections(data)
				gotest.NoError(it, err, doc)
				gotest.NotEmpty(it, marks, "%s holds no generated section; remove it from docref.Documents or mark one", doc)
			}
		})
		w.It("no other document holds one", func(it *gotest.T) {
			var stray []string
			err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if d.IsDir() {
					switch d.Name() {
					case ".git", "node_modules", ".worktrees":
						return filepath.SkipDir
					}
					return nil
				}
				if !strings.HasSuffix(p, ".md") && !strings.HasSuffix(p, ".html") {
					return nil
				}
				rel, err := filepath.Rel(root, p)
				if err != nil {
					return err
				}
				rel = filepath.ToSlash(rel)
				if slices.Contains(docref.Documents, rel) {
					return nil
				}
				data, err := os.ReadFile(p) //nolint:gosec // G304: a document of this repository
				if err != nil {
					return err
				}
				if marks, err := docref.Sections(data); err != nil || len(marks) > 0 {
					stray = append(stray, rel+" holds a gotest:generated marker and is not in docref.Documents; add it")
				}
				return nil
			})
			gotest.NoError(it, err)
			gotest.Empty(it, stray)
		})
	})
}
