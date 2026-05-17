package search

import "github.com/mvrahden/go-test/pkg/gotest"

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
