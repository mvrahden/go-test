package metachars

import "github.com/mvrahden/go-test/pkg/gotest"

// Behavior descriptions are prose, and prose contains punctuation that go
// test's -run reads as a regular expression. Every metacharacter in Go's
// QuoteMeta set appears here, so that addressing one of these behaviors
// exercises the escaping rather than a happy-path name.
type MetacharsTestSuite struct{}

func (s *MetacharsTestSuite) TestPunctuation(t *gotest.T) {
	t.When("a description has {braces}", func(t *gotest.T) {
		t.It("still addresses (parens) and a.dot", func(t *gotest.T) {
			gotest.Equal(t, 1, 1)
		})
		t.It("still addresses [brackets] and a+plus", func(t *gotest.T) {
			gotest.Equal(t, 1, 1)
		})
		t.It("still addresses a*star and a|pipe", func(t *gotest.T) {
			gotest.Equal(t, 1, 1)
		})
		t.It("still addresses ^caret and dollar$", func(t *gotest.T) {
			gotest.Equal(t, 1, 1)
		})
		t.It("still addresses a?question and a-dash", func(t *gotest.T) {
			gotest.Equal(t, 1, 1)
		})
		// A slash is different in kind: go test splits -run on "/" before
		// compiling each element as a regex, so no amount of escaping can
		// address a behavior whose own name contains one.
		t.It("handles https:// URIs", func(t *gotest.T) {
			gotest.Equal(t, 1, 1)
		})
	})
}
