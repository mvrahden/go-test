// Package specmarkdown declares labels that Markdown would read as syntax, Its
// whose labels hold a slash, and suites and methods X_ excludes.
package specmarkdown

import "github.com/mvrahden/go-test/pkg/gotest"

type LabelsTestSuite struct{}

type row struct{ Desc string }

func (s *LabelsTestSuite) TestMarkup(t *gotest.T) {
	t.When("a label holds a | b", func(w *gotest.T) {
		w.It("keeps <b>bold</b> and `code` as text", func(it *gotest.T) {})
		w.It(`keeps a\b and a
line break in one row`, func(it *gotest.T) {})
	})
	t.When("a label holds a [link](http://x)", func(w *gotest.T) {
		w.It("keeps *stars*, _underscores_ and &#124; as written", func(it *gotest.T) {})
	})
	for it, r := range gotest.Each(t, []row{{"row | <i>x</i>"}}) {
		gotest.NotEmpty(it, r.Desc)
	}
}

func (s *LabelsTestSuite) TestPaths(t *gotest.T) {
	t.When("a path is read", func(w *gotest.T) {
		w.It("reads a/b paths", func(it *gotest.T) {})
	})
	t.When("a path is read after its parent", func(w *gotest.T) {
		w.It("reads a", func(it *gotest.T) {})
		w.It("reads a/b", func(it *gotest.T) {})
	})
}

func (s *LabelsTestSuite) X_TestParked(t *gotest.T) {
	t.When("the method is parked", func(w *gotest.T) {
		w.It("never runs", func(it *gotest.T) {})
	})
}

type X_ParkedTestSuite struct{}

func (s *X_ParkedTestSuite) TestAnything(t *gotest.T) {
	t.When("the suite is parked", func(w *gotest.T) {
		w.It("never runs", func(it *gotest.T) {})
	})
}
