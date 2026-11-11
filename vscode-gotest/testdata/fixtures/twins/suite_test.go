package twins

import "github.com/mvrahden/go-test/pkg/gotest"

// The internal half of a suite whose name the external test package
// declares too: both run in one test binary, the external one second.
type TwinsTestSuite struct{}

func (s *TwinsTestSuite) TestCreate(t *gotest.T) {
	t.It("creates inside", func(t *gotest.T) { gotest.Equal(t, 1, 1) })
}

func (s *TwinsTestSuite) TestRead(t *gotest.T) {
	t.It("reads inside", func(t *gotest.T) { gotest.Equal(t, 1, 1) })
}

// Labels where one extends its sibling's: "." finishes before "./pkg"
// starts, so "./pkg" is no child of it.
type PathsTestSuite struct{}

func (s *PathsTestSuite) TestResolve(t *gotest.T) {
	t.It(".", func(t *gotest.T) { gotest.Equal(t, 1, 1) })
	t.It("./pkg", func(t *gotest.T) { gotest.Equal(t, 1, 1) })
}
