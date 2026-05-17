package search_test

import "github.com/mvrahden/go-test/pkg/gotest"

type SearchResultTestSuite struct {
	results []string
}

func (s *SearchResultTestSuite) BeforeEach(t *gotest.T) {
	s.results = []string{}
}

func (s *SearchResultTestSuite) TestEmptyResults(t *gotest.T) {
	t.When("no results are present", func(t *gotest.T) {
		t.It("has zero results", func(t *gotest.T) {
			gotest.Zero(t, len(s.results))
		})
		t.It("is empty", func(t *gotest.T) {
			gotest.Empty(t, s.results)
		})
	})
}

func (s *SearchResultTestSuite) TestCollectResults(t *gotest.T) {
	t.When("results are collected", func(t *gotest.T) {
		s.results = append(s.results, "Go", "Rust", "Python")

		t.It("has a non-zero count", func(t *gotest.T) {
			gotest.NotZero(t, len(s.results))
		})
		t.It("contains the expected items in any order", func(t *gotest.T) {
			gotest.ElementsMatch(t, []string{"Rust", "Python", "Go"}, s.results)
		})
		t.It("does not contain unlisted items", func(t *gotest.T) {
			gotest.NotContains(t, s.results, "Java")
		})
	})
}
