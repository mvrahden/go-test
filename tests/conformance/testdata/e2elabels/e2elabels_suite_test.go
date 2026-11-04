package e2elabels

import "github.com/mvrahden/go-test/pkg/gotest"

// KeysTestSuite declares labels with underscores, which go test's subtest
// names cannot tell from the spaces it replaced, and a When that opens with
// a connective of its own.
type KeysTestSuite struct{}

func (s *KeysTestSuite) TestEncode(t *gotest.T) {
	t.When("the payload uses snake_case", func(w *gotest.T) {
		w.It("returns snake_case keys", func(it *gotest.T) { gotest.Equal(it, 1, 1) })
		w.It("leaves CamelCase alone", func(it *gotest.T) { gotest.Equal(it, 1, 1) })
	})
	t.When("with an empty payload", func(w *gotest.T) {
		w.It("returns no keys", func(it *gotest.T) { gotest.Equal(it, 1, 1) })
	})
}
