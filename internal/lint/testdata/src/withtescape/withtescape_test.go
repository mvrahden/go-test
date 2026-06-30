package withtescape

import (
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type EscapeTestSuite struct{}

func (s *EscapeTestSuite) TestMethodEscape(t *gotest.T) {
	t.T().Errorf("msg")          // want `Errorf is available on gotest.T — unnecessary T escape`
	t.T().FailNow()              // want `FailNow is available on gotest.T — unnecessary T escape`
	t.T().Skip()                 // want `must use Skipf instead — unnecessary T escape`
	t.T().SkipNow()              // want `must use Skipf instead — unnecessary T escape`
	t.T().Skipf("reason")        // want `Skipf is available on gotest.T — unnecessary T escape`
	t.T().Setenv("KEY", "VALUE") // want `Setenv is available on gotest.T — unnecessary T escape`
	_ = t.T().TempDir()          // want `TempDir is available on gotest.T — unnecessary T escape`
}

func (s *EscapeTestSuite) TestAliasEscape(t *gotest.T) {
	tt := t.T()
	tt.Errorf("msg")      // want `Errorf is available on gotest.T — unnecessary T escape`
	tt.Skip()             // want `must use Skipf instead — unnecessary T escape`
	tt.SkipNow()          // want `must use Skipf instead — unnecessary T escape`
	gotest.True(tt, true) // want `pass gotest.T directly to True — unnecessary T escape`
}

func (s *EscapeTestSuite) TestAssertionEscape(t *gotest.T) {
	gotest.True(t.T(), true)  // want `pass gotest.T directly to True — unnecessary T escape`
	gotest.Equal(t.T(), 1, 2) // want `pass gotest.T directly to Equal — unnecessary T escape`
}

func (s *EscapeTestSuite) TestNoEscape(t *gotest.T) {
	t.Errorf("msg")
	gotest.True(t, true)
}

func (s *EscapeTestSuite) BeforeEach() {}
func (s *EscapeTestSuite) AfterEach()  {}

func (s *EscapeTestSuite) TestInterproceduralTestingT(t *gotest.T) {
	helperErrorf(t.T())
}

func (s *EscapeTestSuite) TestInterproceduralGotestT(t *gotest.T) {
	helperGotestErrorf(t)
}

func (s *EscapeTestSuite) TestInterproceduralChain(t *gotest.T) {
	wrapperErrorf(t)
}

func (s *EscapeTestSuite) TestInterproceduralMultiMethod(t *gotest.T) {
	helperMultiMethod(t.T())
}

func (s *EscapeTestSuite) TestInterproceduralNonFirstParam(t *gotest.T) {
	helperNonFirstParam("ctx", t.T())
}

func (s *EscapeTestSuite) TestInterproceduralDeepChain(t *gotest.T) {
	deepOuterErrorf(t.T())
}

func (s *EscapeTestSuite) TestInterproceduralMethodHelper(t *gotest.T) {
	var h escapeHelper
	h.doErrorf(t.T())
}

func standaloneEscape(t *gotest.T) { //nolint:unused
	t.T().Errorf("msg") // want `Errorf is available on gotest.T — unnecessary T escape`
}

func standaloneAssertionEscape(t *gotest.T) { //nolint:unused
	gotest.True(t.T(), true) // want `pass gotest.T directly to True — unnecessary T escape`
}

// Standalone functions must not be flagged for suite-only rules.
func standaloneCleanup(t *gotest.T) { //nolint:unused
	t.T().Cleanup(func() {})
}

func standaloneParallel(t *gotest.T) { //nolint:unused
	t.T().Parallel()
}

func standaloneRun(t *gotest.T) { //nolint:unused
	t.T().Run("sub", func(st *testing.T) {})
}

func helperErrorf(t *testing.T) {
	t.Errorf("msg") // want `Errorf is available on gotest.T — unnecessary T escape`
}

func helperGotestErrorf(t *gotest.T) {
	t.T().Errorf("msg") // want `Errorf is available on gotest.T — unnecessary T escape`
}

func wrapperErrorf(t *gotest.T) {
	helperErrorf(t.T())
}

func helperMultiMethod(t *testing.T) {
	t.FailNow()              // want `FailNow is available on gotest.T — unnecessary T escape`
	t.Skip()                 // want `must use Skipf instead — unnecessary T escape`
	t.SkipNow()              // want `must use Skipf instead — unnecessary T escape`
	t.Skipf("reason")        // want `Skipf is available on gotest.T — unnecessary T escape`
	t.Setenv("KEY", "VALUE") // want `Setenv is available on gotest.T — unnecessary T escape`
	_ = t.TempDir()          // want `TempDir is available on gotest.T — unnecessary T escape`
}

func helperNonFirstParam(name string, t *testing.T) {
	t.Errorf("msg") // want `Errorf is available on gotest.T — unnecessary T escape`
}

func deepLeafErrorf(t *testing.T) {
	t.Errorf("msg") // want `Errorf is available on gotest.T — unnecessary T escape`
}

func deepMiddleErrorf(t *testing.T) {
	deepLeafErrorf(t)
}

func deepOuterErrorf(t *testing.T) {
	deepMiddleErrorf(t)
}

type escapeHelper struct{}

func (h *escapeHelper) doErrorf(t *testing.T) {
	t.Errorf("msg") // want `Errorf is available on gotest.T — unnecessary T escape`
}
