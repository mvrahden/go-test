package duplicates

import "github.com/mvrahden/go-test/pkg/gotest"

// Two rules of the testing package are invisible in the source and decide what
// a behavior is called at run time: a description repeated among its siblings
// is numbered, and a description containing a single slash becomes two subtest
// levels. Discovery has to predict both, or the declared item and the observed
// one are two tree nodes instead of one.
type DuplicatesTestSuite struct{}

func (s *DuplicatesTestSuite) TestRepeatedDescriptions(t *gotest.T) {
	t.When("the same words", func(t *gotest.T) {
		t.It("names the first", func(t *gotest.T) { gotest.Equal(t, 1, 1) })
	})
	t.When("the same words", func(t *gotest.T) {
		t.It("names the second", func(t *gotest.T) { gotest.Equal(t, 1, 1) })
	})
	t.When("the same words", func(t *gotest.T) {
		t.It("names the third", func(t *gotest.T) { gotest.Equal(t, 1, 1) })
	})
}

func (s *DuplicatesTestSuite) TestSlashGrouping(t *gotest.T) {
	t.When("a/b grouping", func(t *gotest.T) {
		t.It("shares the first level", func(t *gotest.T) { gotest.Equal(t, 1, 1) })
	})
	t.When("a/c grouping", func(t *gotest.T) {
		t.It("shares it too", func(t *gotest.T) { gotest.Equal(t, 1, 1) })
	})
}
