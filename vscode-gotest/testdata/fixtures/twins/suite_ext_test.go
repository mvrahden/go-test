package twins_test

import "github.com/mvrahden/go-test/pkg/gotest"

// The external half of the twins: a shared method and one of its own.
type TwinsTestSuite struct{}

func (s *TwinsTestSuite) TestCreate(t *gotest.T) {
	t.It("creates outside", func(t *gotest.T) { gotest.Equal(t, 1, 1) })
}

func (s *TwinsTestSuite) TestList(t *gotest.T) {
	t.It("lists outside", func(t *gotest.T) { gotest.Equal(t, 1, 1) })
}
