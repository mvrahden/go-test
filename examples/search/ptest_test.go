package search

import (
	"strings"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// --- Domain types ---

type Indexable interface {
	comparable
	SearchText() string
	Label() string
}

type Article struct {
	Title string
	Body  string
}

func (a Article) SearchText() string { return a.Title + " " + a.Body }
func (a Article) Label() string      { return a.Title }

type Product struct {
	Name        string
	Description string
}

func (p Product) SearchText() string { return p.Name + " " + p.Description }
func (p Product) Label() string      { return p.Name }

type index[T Indexable] struct {
	items []T
}

func newIndex[T Indexable](items ...T) *index[T] {
	return &index[T]{items: items}
}

func (idx *index[T]) Search(query string) []T {
	q := strings.ToLower(query)
	var results []T
	for _, it := range idx.items {
		if strings.Contains(strings.ToLower(it.SearchText()), q) {
			results = append(results, it)
		}
	}
	return results
}

func (idx *index[T]) Labels() []string {
	ls := make([]string, len(idx.items))
	for i, it := range idx.items {
		ls[i] = it.Label()
	}
	return ls
}

func (idx *index[T]) All() []T {
	out := make([]T, len(idx.items))
	copy(out, idx.items)
	return out
}

// --- Concrete article search suite (parallel, returning BeforeEach) ---

type articleCtx struct {
	idx *index[Article]
	all []Article
}

type ArticleSearchTestSuite struct{}

func (s *ArticleSearchTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *ArticleSearchTestSuite) BeforeEach(t *gotest.T) *articleCtx {
	articles := []Article{
		{"Go Concurrency Patterns", "goroutines and channels"},
		{"Generics in Go", "type parameters and constraints"},
		{"Testing in Go", "test suites and assertions"},
		{"Go Performance", "profiling and benchmarks"},
	}
	return &articleCtx{idx: newIndex(articles...), all: articles}
}

func (s *ArticleSearchTestSuite) TestSearchByTitle(t *gotest.T, ctx *articleCtx) {
	t.When("searching for a title keyword", func(t *gotest.T) {
		results := ctx.idx.Search("generics")

		t.It("finds the matching article", func(t *gotest.T) {
			gotest.Len(t, results, 1)
			gotest.Equal(t, "Generics in Go", results[0].Title)
		})
	})
}

func (s *ArticleSearchTestSuite) TestSearchByBody(t *gotest.T, ctx *articleCtx) {
	t.When("searching for a body keyword", func(t *gotest.T) {
		results := ctx.idx.Search("goroutines")

		t.It("returns articles matching the body text", func(t *gotest.T) {
			gotest.Contains(t, results, ctx.all[0])
		})
	})
}

func (s *ArticleSearchTestSuite) TestSearchMultipleResults(t *gotest.T, ctx *articleCtx) {
	t.When("the query matches multiple articles", func(t *gotest.T) {
		results := ctx.idx.Search("go")

		t.It("returns all matching articles in any order", func(t *gotest.T) {
			gotest.ElementsMatch(t, ctx.all, results)
		})
		t.It("includes a known subset", func(t *gotest.T) {
			gotest.Subset(t, results, []Article{ctx.all[0], ctx.all[2]})
		})
	})
}

func (s *ArticleSearchTestSuite) TestSearchNoResults(t *gotest.T, ctx *articleCtx) {
	t.When("searching for a non-existent term", func(t *gotest.T) {
		results := ctx.idx.Search("zzz_nonexistent_zzz")

		t.It("returns an empty list", func(t *gotest.T) {
			gotest.Empty(t, results)
		})
	})
}

func (s *ArticleSearchTestSuite) TestAllLabels(t *gotest.T, ctx *articleCtx) {
	t.When("listing all labels", func(t *gotest.T) {
		labels := ctx.idx.Labels()

		t.It("includes every article title", func(t *gotest.T) {
			gotest.ElementsMatch(t, []string{
				"Go Concurrency Patterns",
				"Generics in Go",
				"Testing in Go",
				"Go Performance",
			}, labels)
		})
	})
}

// --- Generic contract suite ---

type IndexContractTestSuite[T Indexable] struct{}

func (s *IndexContractTestSuite[T]) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *IndexContractTestSuite[T]) TestEmptyIndex(t *gotest.T) {
	t.When("the index has no items", func(t *gotest.T) {
		idx := newIndex[T]()

		t.It("returns no search results", func(t *gotest.T) {
			gotest.Empty(t, idx.Search("anything"))
		})
		t.It("has no labels", func(t *gotest.T) {
			gotest.Empty(t, idx.Labels())
		})
		t.It("reports zero items", func(t *gotest.T) {
			gotest.Zero(t, len(idx.All()))
		})
	})
}

type ArticleIndexTestSuite = IndexContractTestSuite[Article]

type ProductIndexTestSuite = IndexContractTestSuite[Product]
