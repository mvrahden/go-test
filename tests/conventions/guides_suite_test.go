package conventions_test

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/mvrahden/go-test/internal/speccatalog"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// GuidesTestSuite: specs/ states each behavior once; the guides explain and
// link a requirement there, and never restate a scenario.
type GuidesTestSuite struct{}

func (s *GuidesTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

// guide is one guide file, by its path from the repository root.
type guide struct {
	path string
	data []byte
}

// loadGuides reads README.md, AGENTS.md, ARCHITECTURE.md, every Markdown file
// under docs/ and skills/, and every README.md under tests/.
func loadGuides(t *gotest.T) (string, []guide) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	gotest.NoError(t, err)
	paths := []string{"README.md", "AGENTS.md", "ARCHITECTURE.md"}
	walk := func(dir string, keep func(name string) bool) {
		err := filepath.WalkDir(filepath.Join(root, dir), func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !keep(d.Name()) {
				return err
			}
			rel, err := filepath.Rel(root, p)
			paths = append(paths, filepath.ToSlash(rel))
			return err
		})
		gotest.NoError(t, err)
	}
	isMarkdown := func(name string) bool { return strings.HasSuffix(name, ".md") }
	walk("docs", isMarkdown)
	walk("skills", isMarkdown)
	walk("tests", func(name string) bool { return name == "README.md" })

	guides := make([]guide, 0, len(paths))
	for _, p := range paths {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p))) //nolint:gosec // G304: the repository's own documents
		gotest.NoError(t, err)
		guides = append(guides, guide{path: p, data: data})
	}
	return root, guides
}

func (s *GuidesTestSuite) TestGuidesRestateNothing(t *gotest.T) {
	_, guides := loadGuides(t)
	var found []string
	for _, g := range guides {
		for _, q := range speccatalog.Quotes(g.path, g.data) {
			found = append(found, fmt.Sprintf("%s:%d: %s", q.File, q.Line, q.Text))
		}
		for _, b := range speccatalog.WhenBullets(g.path, g.data) {
			found = append(found, fmt.Sprintf("%s:%d: %s", b.File, b.Line, b.Text))
		}
	}
	t.When("the guides are read outside code fences", func(w *gotest.T) {
		w.It("hold no clause quote and no WHEN bullet", func(it *gotest.T) {
			gotest.Empty(it, found, "state the behavior in specs/ and link the requirement:\n%s", strings.Join(found, "\n"))
		})
	})
}

func (s *GuidesTestSuite) TestLinksIntoSpecsResolve(t *gotest.T) {
	root, guides := loadGuides(t)
	anchors := map[string]map[string]bool{}
	var broken []string
	for _, g := range guides {
		for _, l := range speccatalog.SpecLinks(g.path, g.data) {
			a, ok := anchors[l.Spec]
			if !ok {
				data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(l.Spec))) //nolint:gosec // G304: a path under specs/
				if err == nil {
					a = speccatalog.Anchors(data)
				}
				anchors[l.Spec] = a
			}
			switch {
			case a == nil:
				broken = append(broken, fmt.Sprintf("%s:%d: %s: the link names no file in specs/; link an existing spec", l.File, l.Line, l.Target))
			case l.Anchor != "" && !a[l.Anchor]:
				broken = append(broken, fmt.Sprintf("%s:%d: %s: the link names no requirement in %s; use the anchor of its heading", l.File, l.Line, l.Target, l.Spec))
			}
		}
	}
	t.When("each link from a guide into specs/ is followed", func(w *gotest.T) {
		w.It("names an existing spec file and a heading in it", func(it *gotest.T) {
			gotest.Empty(it, broken, "fix the link:\n%s", strings.Join(broken, "\n"))
		})
	})
}
