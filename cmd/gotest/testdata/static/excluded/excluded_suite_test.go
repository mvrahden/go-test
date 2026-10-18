// Package excluded holds a suite the X_ prefix disables; its behaviors must
// not read as sentences of the contract.
package excluded

import "github.com/mvrahden/go-test/pkg/gotest"

type X_ParkedTestSuite struct{}

func (s *X_ParkedTestSuite) TestParked(t *gotest.T) {
	t.When("the suite is parked", func(w *gotest.T) {
		w.It("is not a sentence anyone may quote", func(it *gotest.T) {
			gotest.True(it, true)
		})
	})
}
