package skipping

import "github.com/mvrahden/go-test/pkg/gotest"

type SkippingTestSuite struct{}

func (s *SkippingTestSuite) TestPending(t *gotest.T) {
	t.When("a capability is not built yet", func(t *gotest.T) {
		t.It("is reported as skipped, not as passing", func(t *gotest.T) {
			t.Skipf("not implemented yet")
		})
		t.It("does not hide the behaviors around it", func(t *gotest.T) {
			gotest.Equal(t, 1, 1)
		})
	})
}
