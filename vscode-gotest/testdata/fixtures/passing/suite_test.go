package passing

import "github.com/mvrahden/go-test/pkg/gotest"

type PassingTestSuite struct{}

func (s *PassingTestSuite) TestArithmetic(t *gotest.T) {
	t.When("adding two numbers", func(t *gotest.T) {
		t.It("returns their sum", func(t *gotest.T) {
			gotest.Equal(t, 4, 2+2)
		})
		t.It("is commutative", func(t *gotest.T) {
			gotest.Equal(t, 2+3, 3+2)
		})
	})
}
