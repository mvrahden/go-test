package gotest_test

import (
	"fmt"
	"testing"

	"github.com/mvrahden/go-test/gotest"
)

// mockT captures assertion failures without stopping execution.
type mockT struct {
	failed  bool
	message string
}

func (m *mockT) Helper()                           {}
func (m *mockT) Errorf(format string, args ...any) { m.failed = true; m.message = fmt.Sprintf(format, args...) }
func (m *mockT) FailNow()                          {}

// --- AssertContext.Equal ---

func TestAssertContext_Equal_passes(t *testing.T) {
	tt := gotest.NewT(t)
	tt.Assert(42).Equal(42)
	tt.Assert("hello").Equal("hello")
	tt.Assert([]int{1, 2}).Equal([]int{1, 2})
}

func TestAssertContext_Equal_fails(t *testing.T) {
	m := &mockT{}
	gotest.NewAssertContext(42, m).Equal(99)
	if !m.failed {
		t.Fatal("Assert(42).Equal(99) should fail")
	}
}

// --- AssertContext.NotEqual ---

func TestAssertContext_NotEqual_passes(t *testing.T) {
	tt := gotest.NewT(t)
	tt.Assert(42).NotEqual(99)
}

func TestAssertContext_NotEqual_fails(t *testing.T) {
	m := &mockT{}
	gotest.NewAssertContext(42, m).NotEqual(42)
	if !m.failed {
		t.Fatal("Assert(42).NotEqual(42) should fail")
	}
}

// --- AssertContext.IsTrue / IsFalse ---

func TestAssertContext_IsTrue_passes(t *testing.T) {
	tt := gotest.NewT(t)
	tt.Assert(true).IsTrue()
}

func TestAssertContext_IsTrue_fails(t *testing.T) {
	m := &mockT{}
	gotest.NewAssertContext(false, m).IsTrue()
	if !m.failed {
		t.Fatal("Assert(false).IsTrue() should fail")
	}
}

func TestAssertContext_IsFalse_passes(t *testing.T) {
	tt := gotest.NewT(t)
	tt.Assert(false).IsFalse()
}

func TestAssertContext_IsFalse_fails(t *testing.T) {
	m := &mockT{}
	gotest.NewAssertContext(true, m).IsFalse()
	if !m.failed {
		t.Fatal("Assert(true).IsFalse() should fail")
	}
}

// --- AssertContext.IsZero / IsNotZero ---

func TestAssertContext_IsZero_passes(t *testing.T) {
	tt := gotest.NewT(t)
	tt.Assert(0).IsZero()
	tt.Assert("").IsZero()
	tt.Assert(false).IsZero()
}

func TestAssertContext_IsZero_fails(t *testing.T) {
	m := &mockT{}
	gotest.NewAssertContext(1, m).IsZero()
	if !m.failed {
		t.Fatal("Assert(1).IsZero() should fail")
	}
}

func TestAssertContext_IsNotZero_passes(t *testing.T) {
	tt := gotest.NewT(t)
	tt.Assert(1).IsNotZero()
	tt.Assert("x").IsNotZero()
}

func TestAssertContext_IsNotZero_fails(t *testing.T) {
	m := &mockT{}
	gotest.NewAssertContext(0, m).IsNotZero()
	if !m.failed {
		t.Fatal("Assert(0).IsNotZero() should fail")
	}
}

// --- AssertContext.NoError / IsError ---

func TestAssertContext_NoError_passes(t *testing.T) {
	tt := gotest.NewT(t)
	tt.Assert(nil).NoError()
	tt.Assert(error(nil)).NoError()
}

func TestAssertContext_NoError_fails(t *testing.T) {
	m := &mockT{}
	gotest.NewAssertContext(fmt.Errorf("boom"), m).NoError()
	if !m.failed {
		t.Fatal("Assert(error).NoError() should fail")
	}
}

func TestAssertContext_IsError_passes(t *testing.T) {
	tt := gotest.NewT(t)
	tt.Assert(fmt.Errorf("boom")).IsError()
}

func TestAssertContext_IsError_fails(t *testing.T) {
	m := &mockT{}
	gotest.NewAssertContext(error(nil), m).IsError()
	if !m.failed {
		t.Fatal("Assert(nil error).IsError() should fail")
	}
}

// --- AssertContext.Empty / NotEmpty ---

func TestAssertContext_Empty_passes(t *testing.T) {
	tt := gotest.NewT(t)
	tt.Assert([]int{}).Empty()
	tt.Assert("").Empty()
	tt.Assert(map[string]int{}).Empty()
}

func TestAssertContext_Empty_fails(t *testing.T) {
	m := &mockT{}
	gotest.NewAssertContext([]int{1}, m).Empty()
	if !m.failed {
		t.Fatal("Assert([]int{1}).Empty() should fail")
	}
}

func TestAssertContext_NotEmpty_passes(t *testing.T) {
	tt := gotest.NewT(t)
	tt.Assert("hello").NotEmpty()
	tt.Assert([]int{1}).NotEmpty()
}

func TestAssertContext_NotEmpty_fails(t *testing.T) {
	m := &mockT{}
	gotest.NewAssertContext("", m).NotEmpty()
	if !m.failed {
		t.Fatal(`Assert("").NotEmpty() should fail`)
	}
}

// --- AssertContext.Contains / NotContains ---

func TestAssertContext_Contains_slice(t *testing.T) {
	tt := gotest.NewT(t)
	tt.Assert([]int{1, 2, 3}).Contains(2)
}

func TestAssertContext_Contains_string(t *testing.T) {
	tt := gotest.NewT(t)
	tt.Assert("hello world").Contains("world")
}

func TestAssertContext_Contains_fails(t *testing.T) {
	m := &mockT{}
	gotest.NewAssertContext([]int{1, 2, 3}, m).Contains(99)
	if !m.failed {
		t.Fatal("Contains(99) should fail")
	}
}

func TestAssertContext_NotContains_passes(t *testing.T) {
	tt := gotest.NewT(t)
	tt.Assert([]int{1, 2}).NotContains(99)
}

// --- AssertContext.HasLength ---

func TestAssertContext_HasLength_passes(t *testing.T) {
	tt := gotest.NewT(t)
	tt.Assert([]int{1, 2, 3}).HasLength(3)
	tt.Assert("abc").HasLength(3)
	tt.Assert(map[string]int{"a": 1}).HasLength(1)
}

func TestAssertContext_HasLength_fails(t *testing.T) {
	m := &mockT{}
	gotest.NewAssertContext([]int{1, 2}, m).HasLength(5)
	if !m.failed {
		t.Fatal("HasLength(5) should fail for len-2 slice")
	}
}

// --- Package-level generic functions ---

func TestPackageLevel_Equal(t *testing.T) {
	gotest.Equal(t, 42, 42)
	gotest.Equal(t, "hello", "hello")
}

func TestPackageLevel_Equal_fails(t *testing.T) {
	m := &mockT{}
	gotest.Equal(m, 42, 99)
	if !m.failed {
		t.Fatal("Equal(42, 99) should fail")
	}
}

func TestPackageLevel_NoError(t *testing.T) {
	gotest.NoError(t, nil)
}

func TestPackageLevel_Error(t *testing.T) {
	gotest.Error(t, fmt.Errorf("boom"))
}

func TestPackageLevel_Greater(t *testing.T) {
	gotest.Greater(t, 2, 1)
	gotest.Greater(t, 2.0, 1.0)
	gotest.Greater(t, "b", "a")
}

func TestPackageLevel_Less(t *testing.T) {
	gotest.Less(t, 1, 2)
}

func TestPackageLevel_Contains(t *testing.T) {
	gotest.Contains(t, "hello world", "world")
	gotest.Contains(t, []int{1, 2, 3}, 2)
}

func TestPackageLevel_Len(t *testing.T) {
	gotest.Len(t, []int{1, 2, 3}, 3)
}

func TestPackageLevel_True(t *testing.T) {
	gotest.True(t, true)
}

func TestPackageLevel_False(t *testing.T) {
	gotest.False(t, false)
}

func TestPackageLevel_Zero(t *testing.T) {
	gotest.Zero(t, 0)
	gotest.Zero(t, "")
}

func TestPackageLevel_NotZero(t *testing.T) {
	gotest.NotZero(t, 1)
	gotest.NotZero(t, "x")
}

func TestPackageLevel_Empty(t *testing.T) {
	gotest.Empty(t, []int{})
}

func TestPackageLevel_NotEmpty(t *testing.T) {
	gotest.NotEmpty(t, []int{1})
}

func TestPackageLevel_ElementsMatch(t *testing.T) {
	gotest.ElementsMatch(t, []int{3, 1, 2}, []int{1, 2, 3})
}

func TestPackageLevel_Must(t *testing.T) {
	val := gotest.Must(42, nil)
	if val != 42 {
		t.Fatalf("expected 42, got %d", val)
	}
}
