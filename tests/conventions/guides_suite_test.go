package conventions_test

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
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
				p := filepath.Join(root, filepath.FromSlash(l.Spec))
				if info, err := os.Stat(p); err == nil && info.IsDir() {
					a = map[string]bool{} // a directory has no anchor
				} else if data, err := os.ReadFile(p); err == nil { //nolint:gosec // G304: a path under specs/
					a = speccatalog.Anchors(data)
				}
				anchors[l.Spec] = a
			}
			switch {
			case a == nil:
				broken = append(broken, fmt.Sprintf("%s:%d: %s: the link names no file or directory in specs/; link an existing spec", l.File, l.Line, l.Target))
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

var (
	// relativeLink finds inline links that are neither absolute nor a bare anchor.
	relativeLink = regexp.MustCompile(`\]\(([^)\s#][^)\s]*)\)`)
	// codeSpan is an inline code span, whose text is not a link.
	codeSpan = regexp.MustCompile("`[^`]*`")
)

func (s *GuidesTestSuite) TestRelativeLinksResolve(t *gotest.T) {
	root, guides := loadGuides(t)
	var broken []string
	for _, g := range guides {
		if strings.HasPrefix(g.path, "tests/") {
			continue
		}
		inFence := false
		for i, line := range strings.Split(string(g.data), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "```") {
				inFence = !inFence
				continue
			}
			if inFence {
				continue
			}
			for _, m := range relativeLink.FindAllStringSubmatch(codeSpan.ReplaceAllString(line, ""), -1) {
				target := m[1]
				if strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:") {
					continue
				}
				path, anchor, _ := strings.Cut(target, "#")
				file := filepath.Join(root, filepath.FromSlash(filepath.Dir(g.path)), filepath.FromSlash(path))
				info, err := os.Stat(file)
				switch {
				case err != nil:
					broken = append(broken, fmt.Sprintf("%s:%d: %s: no such file", g.path, i+1, target))
				case anchor != "" && !info.IsDir() && strings.HasSuffix(path, ".md"):
					data, err := os.ReadFile(file) //nolint:gosec // G304: a document of this repository
					if err != nil || !speccatalog.Anchors(data)[anchor] {
						broken = append(broken, fmt.Sprintf("%s:%d: %s: the file has no such heading", g.path, i+1, target))
					}
				}
			}
		}
	}
	t.When("each relative link of a guide is followed", func(w *gotest.T) {
		w.It("names an existing file and, with an anchor, a heading in it", func(it *gotest.T) {
			gotest.Empty(it, broken, "fix the link:\n%s", strings.Join(broken, "\n"))
		})
	})
}
