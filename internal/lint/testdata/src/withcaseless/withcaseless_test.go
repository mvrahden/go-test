package withcaseless

import "github.com/mvrahden/go-test/pkg/gotest"

type HelperTestSuite struct{ prefix string } // want `suite HelperTestSuite declares no Test, Benchmark or Fuzz method; declare one, or give the helper type a name that does not end in TestSuite`

func (h *HelperTestSuite) name(n string) string { return h.prefix + n }

// SuppressedTestSuite is refused too; the directive silences the report.
type SuppressedTestSuite struct{} //nolint:generation-rejects

type ExcludedTestSuite struct{}

func (s *ExcludedTestSuite) X_TestLater(t *gotest.T) {}

type RealTestSuite struct{}

func (s *RealTestSuite) TestNames(t *gotest.T) {
	h := HelperTestSuite{prefix: "x-"}
	gotest.Equal(t, "x-a", h.name("a"))
}
