package suitelabel_test

import "github.com/mvrahden/go-test/pkg/gotest"

// NamesTestSuite declares an It and a table row whose names end in TestSuite
// and Fixture: labels, not suites or fixtures.
type NamesTestSuite struct{}

func (s *NamesTestSuite) TestLabels(t *gotest.T) {
	t.When("a type is no suite", func(w *gotest.T) {
		w.It("is told to end its name in TestSuite", func(it *gotest.T) {
			gotest.Fail(it, "fails on purpose")
		})
		w.It("is told to bind a DBFixture", func(it *gotest.T) {})
	})
}

func (s *NamesTestSuite) TestRows(t *gotest.T) {
	rows := []struct{ Name string }{{Name: "UserTestSuite"}}
	for it, row := range gotest.Each(t, rows) {
		gotest.NotEmpty(it, row.Name)
	}
}
