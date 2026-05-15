package focusexclude_test

import "github.com/mvrahden/go-test/pkg/gotest"

// NormalExtTestSuite is a regular external-package suite with no focus or
// exclude modifiers.
type NormalExtTestSuite struct {
	value int
}

func (s *NormalExtTestSuite) BeforeEach(t *gotest.T) {
	s.value = 1
}

func (s *NormalExtTestSuite) TestAlpha(t *gotest.T) {
	gotest.Equal(t, 1, s.value)
}

// F_FocusedExtTestSuite is a focused external-package suite; when any F_
// suite exists, only focused suites run and unfocused suites are skipped.
type F_FocusedExtTestSuite struct {
	ready bool
}

func (s *F_FocusedExtTestSuite) BeforeEach(t *gotest.T) {
	s.ready = true
}

func (s *F_FocusedExtTestSuite) TestBeta(t *gotest.T) {
	gotest.True(t, s.ready)
}

// X_TestGamma is excluded and is always skipped regardless of focus state.
func (s *F_FocusedExtTestSuite) X_TestGamma(t *gotest.T) {
	gotest.True(t, s.ready)
}

// X_ExcludedExtTestSuite is excluded and is always skipped.
type X_ExcludedExtTestSuite struct {
	count int
}

func (s *X_ExcludedExtTestSuite) BeforeEach(t *gotest.T) {
	s.count = 42
}

func (s *X_ExcludedExtTestSuite) TestDelta(t *gotest.T) {
	gotest.Equal(t, 42, s.count)
}
