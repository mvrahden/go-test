package focusexclude_test

import (
	focusexclude "github.com/mvrahden/go-test/examples/focus_exclude"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// NormalExtTestSuite is a regular external-package suite with no focus or
// exclude modifiers.
type NormalExtTestSuite struct{}

func (s *NormalExtTestSuite) BeforeEach(t *gotest.T) {
	focusexclude.Noop()
}

func (s *NormalExtTestSuite) TestAlpha(t *gotest.T) {
	focusexclude.Noop()
}

// F_FocusedExtTestSuite is a focused external-package suite; when any F_
// suite exists, only focused suites run and unfocused suites are skipped.
type F_FocusedExtTestSuite struct{}

func (s *F_FocusedExtTestSuite) BeforeEach(t *gotest.T) {
	focusexclude.Noop()
}

func (s *F_FocusedExtTestSuite) TestBeta(t *gotest.T) {
	focusexclude.Noop()
}

// X_TestGamma is excluded and is always skipped regardless of focus state.
func (s *F_FocusedExtTestSuite) X_TestGamma(t *gotest.T) {
	focusexclude.Noop()
}

// X_ExcludedExtTestSuite is excluded and is always skipped.
type X_ExcludedExtTestSuite struct{}

func (s *X_ExcludedExtTestSuite) BeforeEach(t *gotest.T) {
	focusexclude.Noop()
}

func (s *X_ExcludedExtTestSuite) TestDelta(t *gotest.T) {
	focusexclude.Noop()
}
