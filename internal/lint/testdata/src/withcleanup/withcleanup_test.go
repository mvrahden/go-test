package withcleanup

import (
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type ResourceTestSuite struct{}

func (s *ResourceTestSuite) TestDirectCleanup(t *gotest.T) {
	t.T().Cleanup(func() {}) // want `use AfterEach or AfterAll for cleanup — T.Cleanup bypasses suite lifecycle`
}

func (s *ResourceTestSuite) TestMultipleCleanups(t *gotest.T) {
	t.T().Cleanup(func() {}) // want `use AfterEach or AfterAll for cleanup — T.Cleanup bypasses suite lifecycle`
	t.T().Cleanup(func() {}) // want `use AfterEach or AfterAll for cleanup — T.Cleanup bypasses suite lifecycle`
}

func (s *ResourceTestSuite) TestIndirectCleanup(t *gotest.T) {
	tt := t.T()
	tt.Cleanup(func() {}) // want `use AfterEach or AfterAll for cleanup — T.Cleanup bypasses suite lifecycle`
}

func (s *ResourceTestSuite) TestHelperTestingT(t *gotest.T) {
	helperDirect(t.T())
}

func (s *ResourceTestSuite) TestHelperTestingTIndirect(t *gotest.T) {
	tt := t.T()
	helperDirect(tt)
}

func (s *ResourceTestSuite) TestNestedHelpers(t *gotest.T) {
	helperNested(t.T())
}

func (s *ResourceTestSuite) TestGoTestTWrapper(t *gotest.T) {
	wrapperGoTestT(t)
}

func (s *ResourceTestSuite) TestHelperCleanupSecondParam(t *gotest.T) {
	helperCleanupSecondParam("ctx", t.T())
}

func (s *ResourceTestSuite) TestDeepNestedCleanup(t *gotest.T) {
	deepOuterCleanup(t.T())
}

func (s *ResourceTestSuite) TestMethodHelperCleanup(t *gotest.T) {
	var h cleanupHelper
	h.doCleanup(t.T())
}

func (s *ResourceTestSuite) TestNoCleanup(t *gotest.T) {
	gotest.True(t, true)
}

func (s *ResourceTestSuite) BeforeEach() {}
func (s *ResourceTestSuite) AfterEach()  {}

// Helper with *gotest.T — not flagged directly; only call sites in suite
// methods are flagged via interprocedural analysis.
func helperGoTestT(t *gotest.T) { //nolint:unused
	t.T().Cleanup(func() {})
}

// Standalone functions must not be flagged for suite-only rules.
func standaloneCleanup(t *testing.T) { //nolint:unused
	t.Cleanup(func() {})
}

func helperDirect(t *testing.T) {
	t.Cleanup(func() {}) // want `use AfterEach or AfterAll for cleanup — T.Cleanup bypasses suite lifecycle`
}

func helperNested(t *testing.T) {
	helperDirect(t)
}

func wrapperGoTestT(t *gotest.T) {
	helperDirect(t.T())
}

func helperCleanupSecondParam(name string, t *testing.T) {
	t.Cleanup(func() {}) // want `use AfterEach or AfterAll for cleanup — T.Cleanup bypasses suite lifecycle`
}

func deepLeafCleanup(t *testing.T) {
	t.Cleanup(func() {}) // want `use AfterEach or AfterAll for cleanup — T.Cleanup bypasses suite lifecycle`
}

func deepMiddleCleanup(t *testing.T) {
	deepLeafCleanup(t)
}

func deepOuterCleanup(t *testing.T) {
	deepMiddleCleanup(t)
}

type cleanupHelper struct{}

func (h *cleanupHelper) doCleanup(t *testing.T) {
	t.Cleanup(func() {}) // want `use AfterEach or AfterAll for cleanup — T.Cleanup bypasses suite lifecycle`
}
