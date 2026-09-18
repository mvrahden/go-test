package search_test

import "github.com/mvrahden/go-test/pkg/gotest"

type SearchResultTestSuite struct{}

func (s *SearchResultTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

type resultsCtx struct {
	results []string
}

func (s *SearchResultTestSuite) BeforeEach(t *gotest.T) *resultsCtx {
	return &resultsCtx{results: []string{}}
}

func (s *SearchResultTestSuite) TestEmptyResults(t *gotest.T, ctx *resultsCtx) {
	t.When("no results are present", func(t *gotest.T) {
		t.It("has zero results", func(t *gotest.T) {
			gotest.Zero(t, len(ctx.results))
		})
		t.It("is empty", func(t *gotest.T) {
			gotest.Empty(t, ctx.results)
		})
	})
}

func (s *SearchResultTestSuite) TestCollectResults(t *gotest.T, ctx *resultsCtx) {
	t.When("results are collected", func(t *gotest.T) {
		ctx.results = append(ctx.results, "Go", "Rust", "Python")

		t.It("has a non-zero count", func(t *gotest.T) {
			gotest.NotZero(t, len(ctx.results))
		})
		t.It("contains the expected items in any order", func(t *gotest.T) {
			gotest.ElementsMatch(t, []string{"Rust", "Python", "Go"}, ctx.results)
		})
		t.It("does not contain unlisted items", func(t *gotest.T) {
			gotest.NotContains(t, ctx.results, "Java")
		})
	})
}
