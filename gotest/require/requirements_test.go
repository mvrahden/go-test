package require

import (
	"errors"
	"testing"
	"time"
)

type MockT struct {
	Failed bool
}

func (MockT) Helper() {}

func (t *MockT) FailNow() {
	t.Failed = true
}

func (t *MockT) Errorf(format string, args ...any) {
	_, _ = format, args
}

func TestEqual(t *testing.T) {
	t.Parallel()

	Equal(t, 1, 1)

	mockT := new(MockT)
	Equal(mockT, 1, 2)
	if !mockT.Failed {
		t.Error("Check should fail")
	}
}

func TestNotEqual(t *testing.T) {
	t.Parallel()

	NotEqual(t, 1, 2)
	mockT := new(MockT)
	NotEqual(mockT, 2, 2)
	if !mockT.Failed {
		t.Error("Check should fail")
	}
}

func TestTrue(t *testing.T) {
	t.Parallel()

	True(t, true)

	mockT := new(MockT)
	True(mockT, false)
	if !mockT.Failed {
		t.Error("Check should fail")
	}
}

func TestFalse(t *testing.T) {
	t.Parallel()

	False(t, false)

	mockT := new(MockT)
	False(mockT, true)
	if !mockT.Failed {
		t.Error("Check should fail")
	}
}

func TestContains(t *testing.T) {
	t.Parallel()

	Contains(t, "Hello World", "Hello")

	mockT := new(MockT)
	Contains(mockT, "Hello World", "Salut")
	if !mockT.Failed {
		t.Error("Check should fail")
	}
}

func TestNotContains(t *testing.T) {
	t.Parallel()

	NotContains(t, "Hello World", "Hello!")

	mockT := new(MockT)
	NotContains(mockT, "Hello World", "Hello")
	if !mockT.Failed {
		t.Error("Check should fail")
	}
}

func TestPanics(t *testing.T) {
	t.Parallel()

	v := Panics(t, func() {
		panic("Panic!")
	})
	if v != "Panic!" {
		t.Errorf("expected panic value %q, got %v", "Panic!", v)
	}

	mockT := new(MockT)
	Panics(mockT, func() {})
	if !mockT.Failed {
		t.Error("Check should fail")
	}
}

func TestNoError(t *testing.T) {
	t.Parallel()

	NoError(t, nil)

	mockT := new(MockT)
	NoError(mockT, errors.New("some error"))
	if !mockT.Failed {
		t.Error("Check should fail")
	}
}

func TestError(t *testing.T) {
	t.Parallel()

	Error(t, errors.New("some error"))

	mockT := new(MockT)
	Error(mockT, nil)
	if !mockT.Failed {
		t.Error("Check should fail")
	}
}

func TestErrorContains(t *testing.T) {
	t.Parallel()

	ErrorContains(t, errors.New("some error: another error"), "some error")

	mockT := new(MockT)
	ErrorContains(mockT, errors.New("some error"), "different error")
	if !mockT.Failed {
		t.Error("Check should fail")
	}
}

func TestEmpty(t *testing.T) {
	t.Parallel()

	Empty(t, "")

	mockT := new(MockT)
	Empty(mockT, "x")
	if !mockT.Failed {
		t.Error("Check should fail")
	}
}

func TestNotEmpty(t *testing.T) {
	t.Parallel()

	NotEmpty(t, "x")

	mockT := new(MockT)
	NotEmpty(mockT, "")
	if !mockT.Failed {
		t.Error("Check should fail")
	}
}

func TestInDelta(t *testing.T) {
	t.Parallel()

	InDelta(t, 1.001, 1, 0.01)

	mockT := new(MockT)
	InDelta(mockT, 1, 2, 0.5)
	if !mockT.Failed {
		t.Error("Check should fail")
	}
}

func TestZero(t *testing.T) {
	t.Parallel()

	Zero(t, "")

	mockT := new(MockT)
	Zero(mockT, "x")
	if !mockT.Failed {
		t.Error("Check should fail")
	}
}

func TestNotZero(t *testing.T) {
	t.Parallel()

	NotZero(t, "x")

	mockT := new(MockT)
	NotZero(mockT, "")
	if !mockT.Failed {
		t.Error("Check should fail")
	}
}

func TestJSONEq_EqualSONString(t *testing.T) {
	t.Parallel()

	mockT := new(MockT)
	JSONEq(mockT, `{"hello": "world", "foo": "bar"}`, `{"hello": "world", "foo": "bar"}`)
	if mockT.Failed {
		t.Error("Check should pass")
	}
}

func TestJSONEq_EquivalentButNotEqual(t *testing.T) {
	t.Parallel()

	mockT := new(MockT)
	JSONEq(mockT, `{"hello": "world", "foo": "bar"}`, `{"foo": "bar", "hello": "world"}`)
	if mockT.Failed {
		t.Error("Check should pass")
	}
}

func TestJSONEq_HashOfArraysAndHashes(t *testing.T) {
	t.Parallel()

	mockT := new(MockT)
	JSONEq(mockT, "{\r\n\t\"numeric\": 1.5,\r\n\t\"array\": [{\"foo\": \"bar\"}, 1, \"string\", [\"nested\", \"array\", 5.5]],\r\n\t\"hash\": {\"nested\": \"hash\", \"nested_slice\": [\"this\", \"is\", \"nested\"]},\r\n\t\"string\": \"foo\"\r\n}",
		"{\r\n\t\"numeric\": 1.5,\r\n\t\"hash\": {\"nested\": \"hash\", \"nested_slice\": [\"this\", \"is\", \"nested\"]},\r\n\t\"string\": \"foo\",\r\n\t\"array\": [{\"foo\": \"bar\"}, 1, \"string\", [\"nested\", \"array\", 5.5]]\r\n}")
	if mockT.Failed {
		t.Error("Check should pass")
	}
}

func TestJSONEq_Array(t *testing.T) {
	t.Parallel()

	mockT := new(MockT)
	JSONEq(mockT, `["foo", {"hello": "world", "nested": "hash"}]`, `["foo", {"nested": "hash", "hello": "world"}]`)
	if mockT.Failed {
		t.Error("Check should pass")
	}
}

func TestJSONEq_HashAndArrayNotEquivalent(t *testing.T) {
	t.Parallel()

	mockT := new(MockT)
	JSONEq(mockT, `["foo", {"hello": "world", "nested": "hash"}]`, `{"foo": "bar", {"nested": "hash", "hello": "world"}}`)
	if !mockT.Failed {
		t.Error("Check should fail")
	}
}

func TestJSONEq_HashesNotEquivalent(t *testing.T) {
	t.Parallel()

	mockT := new(MockT)
	JSONEq(mockT, `{"foo": "bar"}`, `{"foo": "bar", "hello": "world"}`)
	if !mockT.Failed {
		t.Error("Check should fail")
	}
}

func TestJSONEq_ActualIsNotJSON(t *testing.T) {
	t.Parallel()

	mockT := new(MockT)
	JSONEq(mockT, `{"foo": "bar"}`, "Not JSON")
	if !mockT.Failed {
		t.Error("Check should fail")
	}
}

func TestJSONEq_ExpectedIsNotJSON(t *testing.T) {
	t.Parallel()

	mockT := new(MockT)
	JSONEq(mockT, "Not JSON", `{"foo": "bar", "hello": "world"}`)
	if !mockT.Failed {
		t.Error("Check should fail")
	}
}

func TestJSONEq_ExpectedAndActualNotJSON(t *testing.T) {
	t.Parallel()

	mockT := new(MockT)
	JSONEq(mockT, "Not JSON", "Not JSON")
	if !mockT.Failed {
		t.Error("Check should fail")
	}
}

func TestJSONEq_ArraysOfDifferentOrder(t *testing.T) {
	t.Parallel()

	mockT := new(MockT)
	JSONEq(mockT, `["foo", {"hello": "world", "nested": "hash"}]`, `[{ "hello": "world", "nested": "hash"}, "foo"]`)
	if !mockT.Failed {
		t.Error("Check should fail")
	}
}

func TestMust_Error_Success(t *testing.T) {
	t.Parallel()

	val := Must("hello", nil)
	if val != "hello" {
		t.Errorf("expected %q, got %q", "hello", val)
	}
}

func TestMust_Error_Failure(t *testing.T) {
	t.Parallel()

	defer func() {
		if r := recover(); r == nil {
			t.Error("Must should panic on error")
		}
	}()
	Must("", errors.New("boom"))
}

func TestMust_Bool_Success(t *testing.T) {
	t.Parallel()

	val := Must(42, true)
	if val != 42 {
		t.Errorf("expected 42, got %d", val)
	}
}

func TestMust_Bool_Failure(t *testing.T) {
	t.Parallel()

	defer func() {
		if r := recover(); r == nil {
			t.Error("Must should panic on false")
		}
	}()
	Must(0, false)
}

func TestMust_UnsupportedType(t *testing.T) {
	t.Parallel()

	defer func() {
		if r := recover(); r == nil {
			t.Error("Must should panic on unsupported type")
		}
	}()
	Must("", 42)
}

func TestMust_MultiReturn(t *testing.T) {
	t.Parallel()

	fn := func() (string, error) { return "ok", nil }
	val := Must(fn())
	if val != "ok" {
		t.Errorf("expected %q, got %q", "ok", val)
	}
}

func TestTimeWithin_Pass(t *testing.T) {
	t.Parallel()

	now := time.Now()
	TimeWithin(t, now, now.Add(50*time.Millisecond), 100*time.Millisecond)
}

func TestTimeWithin_Fail(t *testing.T) {
	t.Parallel()

	mockT := new(MockT)
	now := time.Now()
	TimeWithin(mockT, now, now.Add(time.Minute), time.Second)
	if !mockT.Failed {
		t.Error("Check should fail")
	}
}

func TestTimeIsNow(t *testing.T) {
	t.Parallel()

	TimeIsNow(t, time.Now(), time.Second)

	mockT := new(MockT)
	TimeIsNow(mockT, time.Now().Add(-time.Hour), time.Second)
	if !mockT.Failed {
		t.Error("Check should fail")
	}
}
