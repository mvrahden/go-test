package focusexclude

import "github.com/mvrahden/go-test/pkg/gotest"

// NormalTestSuite is a regular suite with no focus or exclude modifiers.
type NormalTestSuite struct {
	value int
}

func (s *NormalTestSuite) BeforeEach(t *gotest.T) {
	s.value = 1
}

func (s *NormalTestSuite) TestAlpha(t *gotest.T) {
	gotest.Equal(t, 1, s.value)
}

// F_FocusedTestSuite is a focused suite; when any F_ suite exists, only
// focused suites run and unfocused suites are skipped.
type F_FocusedTestSuite struct {
	ready bool
}

func (s *F_FocusedTestSuite) BeforeEach(t *gotest.T) {
	s.ready = true
}

func (s *F_FocusedTestSuite) TestBeta(t *gotest.T) {
	gotest.True(t, s.ready)
}

// X_TestGamma is excluded and is always skipped regardless of focus state.
func (s *F_FocusedTestSuite) X_TestGamma(t *gotest.T) {
	gotest.True(t, s.ready)
}

// X_ExcludedTestSuite is excluded and is always skipped.
type X_ExcludedTestSuite struct {
	count int
}

func (s *X_ExcludedTestSuite) BeforeEach(t *gotest.T) {
	s.count = 42
}

func (s *X_ExcludedTestSuite) TestDelta(t *gotest.T) {
	gotest.Equal(t, 42, s.count)
}
