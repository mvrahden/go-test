package gotest_test

import (
	"fmt"
	"testing"

	"github.com/mvrahden/go-test/gotest"
)

func TestAssert_IsTrue_passes_for_true(t *testing.T) {
	tt := gotest.NewT(t)
	tt.Assert(true).IsTrue()
}

func TestAssert_IsTrue_fails_for_false(t *testing.T) {
	var failed bool
	ctx := gotest.NewAssertContext(false, func(format string, args ...any) { failed = true })
	ctx.IsTrue()
	if !failed {
		t.Fatal("Assert(false).IsTrue() should fail")
	}
}

func TestAssert_IsFalse_passes_for_false(t *testing.T) {
	tt := gotest.NewT(t)
	tt.Assert(false).IsFalse()
}

func TestAssert_IsFalse_fails_for_true(t *testing.T) {
	var failed bool
	ctx := gotest.NewAssertContext(true, func(format string, args ...any) { failed = true })
	ctx.IsFalse()
	if !failed {
		t.Fatal("Assert(true).IsFalse() should fail")
	}
}

func TestAssert_Equals_passes_for_equal_values(t *testing.T) {
	tt := gotest.NewT(t)
	tt.Assert(42).Equals(42)
	tt.Assert("hello").Equals("hello")
	tt.Assert([]int{1, 2}).Equals([]int{1, 2})
}

func TestAssert_Equals_fails_for_unequal_values(t *testing.T) {
	var failed bool
	ctx := gotest.NewAssertContext(42, func(format string, args ...any) { failed = true })
	ctx.Equals(99)
	if !failed {
		t.Fatal("Assert(42).Equals(99) should fail")
	}
}

func TestAssert_IsNil_passes_for_nil(t *testing.T) {
	tt := gotest.NewT(t)
	tt.Assert(nil).IsNil()
	var p *int
	tt.Assert(p).IsNil()
	tt.Assert(error(nil)).IsNil()
}

func TestAssert_IsNil_fails_for_non_nil(t *testing.T) {
	var failed bool
	ctx := gotest.NewAssertContext(42, func(format string, args ...any) { failed = true })
	ctx.IsNil()
	if !failed {
		t.Fatal("Assert(42).IsNil() should fail")
	}
}

func TestAssert_IsNotNil_passes_for_non_nil(t *testing.T) {
	tt := gotest.NewT(t)
	tt.Assert(42).IsNotNil()
	tt.Assert("hello").IsNotNil()
	tt.Assert(fmt.Errorf("err")).IsNotNil()
}

func TestAssert_IsNotNil_fails_for_nil(t *testing.T) {
	var failed bool
	ctx := gotest.NewAssertContext(nil, func(format string, args ...any) { failed = true })
	ctx.IsNotNil()
	if !failed {
		t.Fatal("Assert(nil).IsNotNil() should fail")
	}
}

func TestAssert_IsZero_passes_for_zero_values(t *testing.T) {
	tt := gotest.NewT(t)
	tt.Assert(0).IsZero()
	tt.Assert("").IsZero()
	tt.Assert(false).IsZero()
}

func TestAssert_IsZero_fails_for_non_zero(t *testing.T) {
	var failed bool
	ctx := gotest.NewAssertContext(1, func(format string, args ...any) { failed = true })
	ctx.IsZero()
	if !failed {
		t.Fatal("Assert(1).IsZero() should fail")
	}
}

func TestAssert_HasLength_passes_for_correct_length(t *testing.T) {
	tt := gotest.NewT(t)
	tt.Assert([]int{1, 2, 3}).HasLength(3)
	tt.Assert("abc").HasLength(3)
	tt.Assert(map[string]int{"a": 1}).HasLength(1)
}

func TestAssert_HasLength_fails_for_wrong_length(t *testing.T) {
	var failed bool
	ctx := gotest.NewAssertContext([]int{1, 2}, func(format string, args ...any) { failed = true })
	ctx.HasLength(5)
	if !failed {
		t.Fatal("Assert([]int{1,2}).HasLength(5) should fail")
	}
}

func TestAssert_IsEmpty_passes_for_empty(t *testing.T) {
	tt := gotest.NewT(t)
	tt.Assert([]int{}).IsEmpty()
	tt.Assert("").IsEmpty()
	tt.Assert(map[string]int{}).IsEmpty()
}

func TestAssert_IsEmpty_fails_for_non_empty(t *testing.T) {
	var failed bool
	ctx := gotest.NewAssertContext([]int{1}, func(format string, args ...any) { failed = true })
	ctx.IsEmpty()
	if !failed {
		t.Fatal("Assert([]int{1}).IsEmpty() should fail")
	}
}

func TestAssert_Contains_passes_for_slice_with_element(t *testing.T) {
	tt := gotest.NewT(t)
	tt.Assert([]int{1, 2, 3}).Contains(2)
}

func TestAssert_Contains_passes_for_string_with_substring(t *testing.T) {
	tt := gotest.NewT(t)
	tt.Assert("hello world").Contains("world")
}

func TestAssert_Contains_fails_for_missing_element(t *testing.T) {
	var failed bool
	ctx := gotest.NewAssertContext([]int{1, 2, 3}, func(format string, args ...any) { failed = true })
	ctx.Contains(99)
	if !failed {
		t.Fatal("Assert([]int{1,2,3}).Contains(99) should fail")
	}
}
