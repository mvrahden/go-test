// Ring 0: raw checks only (see ring0_suite_test.go).
package assert_test //nolint:fail-guard

import (
	"math"
	"strings"

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

func (s *CompareTestSuite) TestCheckInDelta_NaNDeltaFailsInShape(t *gotest.T, _ *ring0Ctx) {
	verdict := assert.CheckInDelta(1.0, 1.0, math.NaN())
	if !strings.HasPrefix(verdict, "InDelta failed:\n  ") || !strings.Contains(verdict, "delta is NaN") {
		t.Errorf("a NaN delta: got %q, want an InDelta failed: message naming the NaN delta", verdict)
	}
}

func (s *CompareTestSuite) TestCheckInDelta_IntegersCompareExactly(t *gotest.T, _ *ring0Ctx) {
	mustFail(t, assert.CheckInDelta(int64(9007199254740993), int64(9007199254740992), 0), "int64 past 2^53, delta 0", "InDelta failed", "= 1 exceeds delta 0")
	mustPass(t, assert.CheckInDelta(int64(9007199254740993), int64(9007199254740992), 1), "int64 past 2^53, delta 1")
	mustFail(t, assert.CheckInDelta(uint64(math.MaxUint64), uint64(math.MaxUint64-1), 0), "uint64 at its top, delta 0", "= 1 exceeds delta 0")
	mustPass(t, assert.CheckInDelta(uint64(math.MaxUint64), uint64(math.MaxUint64), 0), "equal uint64 at its top")
	mustFail(t, assert.CheckInDelta(int64(1), int64(1), -0.5), "a negative delta", "InDelta failed")
	mustPass(t, assert.CheckInDelta(int64(3), int64(5), 2.5), "a fractional delta above the distance")
	mustFail(t, assert.CheckInDelta(int64(3), int64(6), 2.5), "a fractional delta below the distance", "= 3 exceeds delta 2.5")
}

func (s *CompareTestSuite) TestCheckInDelta_DifferenceDoesNotOverflow(t *gotest.T, _ *ring0Ctx) {
	mustFail(t, assert.CheckInDelta(int64(math.MinInt64), int64(math.MaxInt64), 1), "int64 extremes, delta 1", "= 18446744073709551615 exceeds delta 1")
	mustPass(t, assert.CheckInDelta(int64(math.MinInt64), int64(math.MaxInt64), math.MaxUint64), "int64 extremes within the full range")
	mustFail(t, assert.CheckInDelta(int8(-128), int8(127), 254), "int8 extremes, delta 254", "= 255 exceeds delta 254")
	mustPass(t, assert.CheckInDelta(int8(127), int8(-128), 255), "int8 extremes, delta 255")
	mustFail(t, assert.CheckInDelta(uint8(0), uint8(255), 254), "uint8 extremes, smaller first", "= 255 exceeds delta 254")
	mustFail(t, assert.CheckInDelta(uint64(0), uint64(math.MaxUint64), 1e19), "uint64 extremes, delta just below the distance", "= 18446744073709551615 exceeds delta 1e+19")
	mustPass(t, assert.CheckInDelta(uint64(0), uint64(math.MaxUint64), math.Inf(1)), "uint64 extremes, infinite delta")
}
