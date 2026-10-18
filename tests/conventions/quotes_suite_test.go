package conventions_test

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/mvrahden/go-test/internal/speccatalog"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// ProseQuotesTestSuite: the documents may state a behavior only by quoting a
// clause of the catalog, so a sentence that stops being true stops building.
type ProseQuotesTestSuite struct{}

// quotedDocs are the documents that quote catalog clauses.
var quotedDocs = []string{
	"docs/design/spec.md",
	"AGENTS.md",
	"README.md",
	"skills/writing-gotest-tests/SKILL.md",
}

func (s *ProseQuotesTestSuite) TestEveryQuoteResolves(t *gotest.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	gotest.NoError(t, err)
	js, err := os.ReadFile(filepath.Join(root, speccatalog.JSONPath))
	gotest.NoError(t, err)
	clauses, err := speccatalog.Clauses(js)
	gotest.NoError(t, err)

	docs := append([]string(nil), quotedDocs...)
	refs, err := filepath.Glob(filepath.Join(root, "skills", "writing-gotest-tests", "reference", "*.md"))
	gotest.NoError(t, err)
	for _, r := range refs {
		rel, err := filepath.Rel(root, r)
		gotest.NoError(t, err)
		docs = append(docs, rel)
	}
	sort.Strings(docs)

	var quotes []speccatalog.Quote
	for _, doc := range docs {
		data, err := os.ReadFile(filepath.Join(root, doc)) //nolint:gosec // G304: the repository's own documents
		gotest.NoError(t, err)
		quotes = append(quotes, speccatalog.Quotes(doc, data)...)
	}

	t.When("every clause quote in the documents is looked up in the catalog", func(w *gotest.T) {
		w.It("resolves", func(it *gotest.T) {
			gotest.Empty(it, speccatalog.Unresolved(quotes, clauses), "a quoted behavior must exist as a conformance clause; run make catalog after adding it")
		})
	})
}
