package nested

import "github.com/mvrahden/go-test/pkg/gotest"

// Exercises deep When nesting, so the spec tree has interior nodes whose own
// verdict has to be derived from descendants rather than reported directly.
type NestedTestSuite struct{}

func (s *NestedTestSuite) TestCheckout(t *gotest.T) {
	t.When("a cart has items", func(t *gotest.T) {
		t.When("and the customer is a member", func(t *gotest.T) {
			t.When("and a coupon applies", func(t *gotest.T) {
				t.It("stacks both discounts", func(t *gotest.T) {
					gotest.Equal(t, 30, 10+20)
				})
			})
			t.It("applies the member discount", func(t *gotest.T) {
				gotest.Equal(t, 10, 10)
			})
		})
		t.It("computes a subtotal", func(t *gotest.T) {
			gotest.True(t, 100 > 0)
		})
	})
}
