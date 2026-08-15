package failing

import "github.com/mvrahden/go-test/pkg/gotest"

type FailingTestSuite struct{}

func (s *FailingTestSuite) TestTotals(t *gotest.T) {
	t.When("summing a basket", func(t *gotest.T) {
		t.It("reports the expected total", func(t *gotest.T) {
			gotest.Equal(t, 300, 250)
		})
		t.It("still runs the sibling behavior", func(t *gotest.T) {
			gotest.Equal(t, 1, 1)
		})
	})
}
