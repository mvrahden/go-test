package simplesuite

import "github.com/mvrahden/go-test/pkg/gotest"

type SimpleTestSuite struct {
	items []string
}

func (s *SimpleTestSuite) BeforeEach(t *gotest.T) {
	s.items = []string{"alpha", "beta", "gamma"}
}

func (s *SimpleTestSuite) TestLength(t *gotest.T) {
	gotest.Len(t, s.items, 3)
}

func (s *SimpleTestSuite) TestContains(t *gotest.T) {
	gotest.Contains(t, s.items, "beta")
}
