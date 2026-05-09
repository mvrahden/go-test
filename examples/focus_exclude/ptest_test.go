package focusexclude

import (
	"github.com/mvrahden/go-test/pkg/gotest"
)

// NormalTestSuite is a regular suite with no focus or exclude modifiers.
type NormalTestSuite struct{}

func (s *NormalTestSuite) BeforeEach(t *gotest.T) {
	Noop()
}

func (s *NormalTestSuite) TestAlpha(t *gotest.T) {
	Noop()
}

// F_FocusedTestSuite is a focused suite; when any F_ suite exists, only
// focused suites run and unfocused suites are skipped.
type F_FocusedTestSuite struct{}

func (s *F_FocusedTestSuite) BeforeEach(t *gotest.T) {
	Noop()
}

func (s *F_FocusedTestSuite) TestBeta(t *gotest.T) {
	Noop()
}

// X_TestGamma is excluded and is always skipped regardless of focus state.
func (s *F_FocusedTestSuite) X_TestGamma(t *gotest.T) {
	Noop()
}

// X_ExcludedTestSuite is excluded and is always skipped.
type X_ExcludedTestSuite struct{}

func (s *X_ExcludedTestSuite) BeforeEach(t *gotest.T) {
	Noop()
}

func (s *X_ExcludedTestSuite) TestDelta(t *gotest.T) {
	Noop()
}
