package withsimplify //nolint:stdlib-test

import (
	"errors"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// === True with binary comparisons ===

func TestTrueComparisons(t *testing.T) {
	a, b := 1, 2
	gotest.True(t, a == b) // want `use Equal instead of True for == comparison`
	gotest.True(t, a != b) // want `use NotEqual instead of True for != comparison`
	gotest.True(t, a > b)  // want `use Greater instead of True for > comparison`
	gotest.True(t, a >= b) // want `use GreaterOrEqual instead of True for >= comparison`
	gotest.True(t, a < b)  // want `use Less instead of True for < comparison`
	gotest.True(t, a <= b) // want `use LessOrEqual instead of True for <= comparison`
}

// === True with nil checks — all nilable type categories ===

func TestTrueNilError(t *testing.T) {
	var err error
	gotest.True(t, err == nil) // want `use NoError instead of True for error nil check`
	gotest.True(t, err != nil) // want `use Error instead of True for error nil check`
	gotest.True(t, nil == err) // want `use NoError instead of True for error nil check`
	gotest.True(t, nil != err) // want `use Error instead of True for error nil check`
}

func TestTrueNilComparable(t *testing.T) {
	// pointer
	var p *int
	gotest.True(t, p == nil) // want `use Zero instead of True for nil check`
	gotest.True(t, p != nil) // want `use NotZero instead of True for nil check`

	// channel
	var ch chan int
	gotest.True(t, ch == nil) // want `use Zero instead of True for nil check`
	gotest.True(t, ch != nil) // want `use NotZero instead of True for nil check`

	// non-error interface
	var iface any
	gotest.True(t, iface == nil) // want `use Zero instead of True for nil check`
	gotest.True(t, iface != nil) // want `use NotZero instead of True for nil check`
}

func TestTrueNilEmptyable(t *testing.T) {
	// slice
	var s []int
	gotest.True(t, s == nil) // want `use Empty instead of True for nil check`
	gotest.True(t, s != nil) // want `use NotEmpty instead of True for nil check`

	// map
	var m map[string]int
	gotest.True(t, m == nil) // want `use Empty instead of True for nil check`
	gotest.True(t, m != nil) // want `use NotEmpty instead of True for nil check`
}

// === True with len checks ===

func TestTrueLen(t *testing.T) {
	s := []int{1, 2, 3}
	gotest.True(t, len(s) == 0) // want `use Empty instead of True for len == 0 check`
	gotest.True(t, len(s) != 0) // want `use NotEmpty instead of True for len == 0 check`
	gotest.True(t, len(s) > 0)  // want `use NotEmpty instead of True for len > 0 check`
	gotest.True(t, len(s) >= 1) // want `use NotEmpty instead of True for len >= 1 check`
	gotest.True(t, len(s) == 3) // want `use Len instead of True for len comparison`
	gotest.True(t, 0 == len(s)) // want `use Empty instead of True for len == 0 check`
	gotest.True(t, 0 != len(s)) // want `use NotEmpty instead of True for len == 0 check`
	gotest.True(t, 3 == len(s)) // want `use Len instead of True for len comparison`
}

// === True with call expressions ===

func TestTrueCalls(t *testing.T) {
	s := "hello world"
	gotest.True(t, strings.Contains(s, "hello"))    // want `use Contains instead of True for strings\.Contains call`
	gotest.True(t, errors.Is(errors.New("x"), nil)) // want `use ErrorIs instead of True for errors\.Is call`
	re := regexp.MustCompile(".*")
	gotest.True(t, re.MatchString("hello")) // want `use Regexp instead of True for MatchString call`
	gotest.True(t, reflect.DeepEqual(1, 2)) // want `use Equal instead of True for reflect\.DeepEqual call`
}

// === True with negation ===

func TestTrueNegation(t *testing.T) {
	b := true
	gotest.True(t, !b) // want `use False instead of True for negation`

	s := "hello"
	gotest.True(t, !strings.Contains(s, "z")) // want `use NotContains instead of True for negated strings\.Contains call`
}

// === False with binary comparisons ===

func TestFalseComparisons(t *testing.T) {
	a, b := 1, 2
	gotest.False(t, a == b) // want `use NotEqual instead of False for == comparison`
	gotest.False(t, a != b) // want `use Equal instead of False for != comparison`
	gotest.False(t, a > b)  // want `use LessOrEqual instead of False for > comparison`
	gotest.False(t, a >= b) // want `use Less instead of False for >= comparison`
	gotest.False(t, a < b)  // want `use GreaterOrEqual instead of False for < comparison`
	gotest.False(t, a <= b) // want `use Greater instead of False for <= comparison`
}

// === False with nil checks — all nilable type categories ===

func TestFalseNilError(t *testing.T) {
	var err error
	gotest.False(t, err == nil) // want `use Error instead of False for error nil check`
	gotest.False(t, err != nil) // want `use NoError instead of False for error nil check`
}

func TestFalseNilComparable(t *testing.T) {
	// pointer
	var p *int
	gotest.False(t, p == nil) // want `use NotZero instead of False for nil check`
	gotest.False(t, p != nil) // want `use Zero instead of False for nil check`

	// channel
	var ch chan int
	gotest.False(t, ch == nil) // want `use NotZero instead of False for nil check`
	gotest.False(t, ch != nil) // want `use Zero instead of False for nil check`

	// non-error interface
	var iface any
	gotest.False(t, iface == nil) // want `use NotZero instead of False for nil check`
	gotest.False(t, iface != nil) // want `use Zero instead of False for nil check`
}

func TestFalseNilEmptyable(t *testing.T) {
	// slice
	var s []int
	gotest.False(t, s == nil) // want `use NotEmpty instead of False for nil check`
	gotest.False(t, s != nil) // want `use Empty instead of False for nil check`

	// map
	var m map[string]int
	gotest.False(t, m == nil) // want `use NotEmpty instead of False for nil check`
	gotest.False(t, m != nil) // want `use Empty instead of False for nil check`
}

// === False with len checks ===

func TestFalseLen(t *testing.T) {
	s := []int{1, 2, 3}
	gotest.False(t, len(s) == 0) // want `use NotEmpty instead of False for len == 0 check`
	gotest.False(t, len(s) != 0) // want `use Empty instead of False for len == 0 check`
	gotest.False(t, len(s) > 0)  // want `use Empty instead of False for len > 0 check`
	gotest.False(t, len(s) >= 1) // want `use Empty instead of False for len >= 1 check`
	gotest.False(t, 0 == len(s)) // want `use NotEmpty instead of False for len == 0 check`
}

// === False with call expressions ===

func TestFalseCalls(t *testing.T) {
	s := "hello world"
	gotest.False(t, strings.Contains(s, "xyz")) // want `use NotContains instead of False for strings\.Contains call`
	gotest.False(t, reflect.DeepEqual(1, 2))    // want `use NotEqual instead of False for reflect\.DeepEqual call`
}

// === False with negation ===

func TestFalseNegation(t *testing.T) {
	b := true
	gotest.False(t, !b) // want `use True instead of False for negation`

	s := "hello"
	gotest.False(t, !strings.Contains(s, "h")) // want `use Contains instead of False for negated strings\.Contains call`
}

// === Equal with special values ===

func TestEqualBoolLiteral(t *testing.T) {
	b := true
	gotest.Equal(t, true, b)  // want `use True instead of Equal for bool literal comparison`
	gotest.Equal(t, false, b) // want `use False instead of Equal for bool literal comparison`
	gotest.Equal(t, b, true)  // want `use True instead of Equal for bool literal comparison`
	gotest.Equal(t, b, false) // want `use False instead of Equal for bool literal comparison`
}

func TestEqualNil(t *testing.T) {
	// error
	var err error
	gotest.Equal(t, nil, err) // want `use NoError instead of Equal for nil error comparison`
	gotest.Equal(t, err, nil) // want `use NoError instead of Equal for nil error comparison`

	// pointer — comparable
	var p *int
	gotest.Equal(t, nil, p) // want `use Zero instead of Equal for nil comparison`
	gotest.Equal(t, p, nil) // want `use Zero instead of Equal for nil comparison`

	// channel — comparable
	var ch chan int
	gotest.Equal(t, nil, ch) // want `use Zero instead of Equal for nil comparison`
	gotest.Equal(t, ch, nil) // want `use Zero instead of Equal for nil comparison`

	// non-error interface — comparable
	var iface any
	gotest.Equal(t, nil, iface) // want `use Zero instead of Equal for nil comparison`
	gotest.Equal(t, iface, nil) // want `use Zero instead of Equal for nil comparison`

	// slice — emptyable
	var s []int
	gotest.Equal(t, nil, s) // want `use Empty instead of Equal for nil comparison`
	gotest.Equal(t, s, nil) // want `use Empty instead of Equal for nil comparison`

	// map — emptyable
	var m map[string]int
	gotest.Equal(t, nil, m) // want `use Empty instead of Equal for nil comparison`
	gotest.Equal(t, m, nil) // want `use Empty instead of Equal for nil comparison`
}

func TestEqualLen(t *testing.T) {
	s := []int{1, 2, 3}
	gotest.Equal(t, len(s), 0) // want `use Empty instead of Equal for len == 0 comparison`
	gotest.Equal(t, 0, len(s)) // want `use Empty instead of Equal for len == 0 comparison`
	gotest.Equal(t, len(s), 3) // want `use Len instead of Equal for len comparison`
	gotest.Equal(t, 3, len(s)) // want `use Len instead of Equal for len comparison`
}

// === NotEqual with special values ===

func TestNotEqualBoolLiteral(t *testing.T) {
	b := true
	gotest.NotEqual(t, true, b)  // want `use False instead of NotEqual for bool literal comparison`
	gotest.NotEqual(t, false, b) // want `use True instead of NotEqual for bool literal comparison`
	gotest.NotEqual(t, b, true)  // want `use False instead of NotEqual for bool literal comparison`
	gotest.NotEqual(t, b, false) // want `use True instead of NotEqual for bool literal comparison`
}

func TestNotEqualNil(t *testing.T) {
	// error
	var err error
	gotest.NotEqual(t, nil, err) // want `use Error instead of NotEqual for nil error comparison`
	gotest.NotEqual(t, err, nil) // want `use Error instead of NotEqual for nil error comparison`

	// pointer — comparable
	var p *int
	gotest.NotEqual(t, nil, p) // want `use NotZero instead of NotEqual for nil comparison`
	gotest.NotEqual(t, p, nil) // want `use NotZero instead of NotEqual for nil comparison`

	// channel — comparable
	var ch chan int
	gotest.NotEqual(t, nil, ch) // want `use NotZero instead of NotEqual for nil comparison`
	gotest.NotEqual(t, ch, nil) // want `use NotZero instead of NotEqual for nil comparison`

	// non-error interface — comparable
	var iface any
	gotest.NotEqual(t, nil, iface) // want `use NotZero instead of NotEqual for nil comparison`
	gotest.NotEqual(t, iface, nil) // want `use NotZero instead of NotEqual for nil comparison`

	// slice — emptyable
	var s []int
	gotest.NotEqual(t, nil, s) // want `use NotEmpty instead of NotEqual for nil comparison`
	gotest.NotEqual(t, s, nil) // want `use NotEmpty instead of NotEqual for nil comparison`

	// map — emptyable
	var m map[string]int
	gotest.NotEqual(t, nil, m) // want `use NotEmpty instead of NotEqual for nil comparison`
	gotest.NotEqual(t, m, nil) // want `use NotEmpty instead of NotEqual for nil comparison`
}

func TestNotEqualLen(t *testing.T) {
	s := []int{1, 2, 3}
	gotest.NotEqual(t, len(s), 0) // want `use NotEmpty instead of NotEqual for len == 0 comparison`
	gotest.NotEqual(t, 0, len(s)) // want `use NotEmpty instead of NotEqual for len == 0 comparison`
}

// === Len with zero ===

func TestLenZero(t *testing.T) {
	s := []int{}
	gotest.Len(t, s, 0) // want `use Empty instead of Len for zero length check`
}

// === Greater / GreaterOrEqual with len ===

func TestGreaterLen(t *testing.T) {
	s := []int{1}
	gotest.Greater(t, len(s), 0)        // want `use NotEmpty instead of Greater for len > 0 check`
	gotest.GreaterOrEqual(t, len(s), 1) // want `use NotEmpty instead of GreaterOrEqual for len >= 1 check`
}

// === Zero / NotZero with error ===

func TestZeroError(t *testing.T) {
	var err error
	gotest.Zero(t, err)    // want `use NoError instead of Zero for error zero check`
	gotest.NotZero(t, err) // want `use Error instead of NotZero for error zero check`
}

// === Contains with err.Error() ===

func TestContainsErrMsg(t *testing.T) {
	err := errors.New("something failed")
	gotest.Contains(t, err.Error(), "failed") // want `use ErrorContains instead of Contains for err\.Error\(\) contains check`
}

// === Correct usage — no diagnostics ===

func TestCorrectUsage(t *testing.T) {
	a, b := 1, 2
	gotest.Equal(t, a, b)
	gotest.NotEqual(t, a, b)
	gotest.Greater(t, a, b)
	gotest.True(t, true)
	gotest.False(t, false)

	var err error
	gotest.NoError(t, err)
	gotest.Error(t, err)

	s := []int{1}
	gotest.Empty(t, s)
	gotest.NotEmpty(t, s)
	gotest.Len(t, s, 3)
	gotest.Contains(t, "hello", "h")

	// func type — not emptyable or comparable, no suggestion possible
	var fn func()
	gotest.True(t, fn == nil)
	gotest.True(t, fn != nil)
	gotest.False(t, fn == nil)
	gotest.Equal(t, nil, fn)
	gotest.NotEqual(t, nil, fn)

	// Len with nil object — semantically different from Empty, no suggestion
	gotest.Len(t, nil, 0)
}

// === With message args — preserved in fix ===

func TestWithMsgArgs(t *testing.T) {
	a, b := 1, 2
	gotest.True(t, a == b, "values should match") // want `use Equal instead of True for == comparison`
}
