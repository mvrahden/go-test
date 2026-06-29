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
	helperDirect(t.T()) // want `use AfterEach or AfterAll for cleanup — T.Cleanup bypasses suite lifecycle`
}

func (s *ResourceTestSuite) TestHelperTestingTIndirect(t *gotest.T) {
	tt := t.T()
	helperDirect(tt) // want `use AfterEach or AfterAll for cleanup — T.Cleanup bypasses suite lifecycle`
}

func (s *ResourceTestSuite) TestNestedHelpers(t *gotest.T) {
	helperNested(t.T()) // want `use AfterEach or AfterAll for cleanup — T.Cleanup bypasses suite lifecycle`
}

func (s *ResourceTestSuite) TestGoTestTWrapper(t *gotest.T) {
	wrapperGoTestT(t) // want `use AfterEach or AfterAll for cleanup — T.Cleanup bypasses suite lifecycle`
}

func (s *ResourceTestSuite) TestNoCleanup(t *gotest.T) {
	gotest.True(t, true)
}

func (s *ResourceTestSuite) BeforeEach() {}
func (s *ResourceTestSuite) AfterEach()  {}

// Helper with *gotest.T — .T().Cleanup() caught by direct detection.
func helperGoTestT(t *gotest.T) { //nolint:unused
	t.T().Cleanup(func() {}) // want `use AfterEach or AfterAll for cleanup — T.Cleanup bypasses suite lifecycle`
}

// Helper with *testing.T — .Cleanup() caught only via transitive analysis.
func helperDirect(t *testing.T) {
	t.Cleanup(func() {})
}

// Nested chain: passes *testing.T to helperDirect.
func helperNested(t *testing.T) {
	helperDirect(t)
}

// Takes *gotest.T, bridges to *testing.T helper via .T().
func wrapperGoTestT(t *gotest.T) {
	helperDirect(t.T())
}
