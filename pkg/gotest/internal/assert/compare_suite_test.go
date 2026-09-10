// Ring 0: raw checks only (see ring0_suite_test.go).
package assert_test //nolint:fail-guard

import (
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/pkg/gotest/internal/assert"
)

// CompareTestSuite covers the ordered comparison kernels.
type CompareTestSuite struct{}

func (s *CompareTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *CompareTestSuite) BeforeEach(t *gotest.T) *ring0Ctx { return &ring0Ctx{} }

func (s *CompareTestSuite) TestCheckGreater_IntGreaterPasses(t *gotest.T, _ *ring0Ctx) {
	mustPass(t, assert.CheckGreater(5, 3), "5 > 3")
}

func (s *CompareTestSuite) TestCheckGreater_IntEqualFails(t *gotest.T, _ *ring0Ctx) {
	mustFail(t, assert.CheckGreater(4, 4), "4 > 4 (equal)", "Greater failed", "not greater than")
}

func (s *CompareTestSuite) TestCheckGreater_IntLessFails(t *gotest.T, _ *ring0Ctx) {
	mustFail(t, assert.CheckGreater(2, 7), "2 > 7", "Greater failed")
}

func (s *CompareTestSuite) TestCheckGreater_FloatPasses(t *gotest.T, _ *ring0Ctx) {
	mustPass(t, assert.CheckGreater(3.14, 2.71), "3.14 > 2.71")
}

func (s *CompareTestSuite) TestCheckGreater_StringPasses(t *gotest.T, _ *ring0Ctx) {
	mustPass(t, assert.CheckGreater("b", "a"), `"b" > "a"`)
}

func (s *CompareTestSuite) TestCheckLess_IntLessPasses(t *gotest.T, _ *ring0Ctx) {
	mustPass(t, assert.CheckLess(1, 10), "1 < 10")
}

func (s *CompareTestSuite) TestCheckLess_IntEqualFails(t *gotest.T, _ *ring0Ctx) {
	mustFail(t, assert.CheckLess(5, 5), "5 < 5 (equal)", "Less failed", "not less than")
}

func (s *CompareTestSuite) TestCheckLess_IntGreaterFails(t *gotest.T, _ *ring0Ctx) {
	mustFail(t, assert.CheckLess(9, 3), "9 < 3", "Less failed")
}

func (s *CompareTestSuite) TestCheckGreaterOrEqual_GreaterPasses(t *gotest.T, _ *ring0Ctx) {
	mustPass(t, assert.CheckGreaterOrEqual(10, 5), "10 >= 5")
}

func (s *CompareTestSuite) TestCheckGreaterOrEqual_EqualPasses(t *gotest.T, _ *ring0Ctx) {
	mustPass(t, assert.CheckGreaterOrEqual(7, 7), "7 >= 7")
}

func (s *CompareTestSuite) TestCheckGreaterOrEqual_LessFails(t *gotest.T, _ *ring0Ctx) {
	mustFail(t, assert.CheckGreaterOrEqual(3, 8), "3 >= 8", "GreaterOrEqual failed", "not greater than or equal to")
}

func (s *CompareTestSuite) TestCheckLessOrEqual_LessPasses(t *gotest.T, _ *ring0Ctx) {
	mustPass(t, assert.CheckLessOrEqual(2, 9), "2 <= 9")
}

func (s *CompareTestSuite) TestCheckLessOrEqual_EqualPasses(t *gotest.T, _ *ring0Ctx) {
	mustPass(t, assert.CheckLessOrEqual(6, 6), "6 <= 6")
}

func (s *CompareTestSuite) TestCheckLessOrEqual_GreaterFails(t *gotest.T, _ *ring0Ctx) {
	mustFail(t, assert.CheckLessOrEqual(11, 4), "11 <= 4", "LessOrEqual failed", "not less than or equal to")
}
