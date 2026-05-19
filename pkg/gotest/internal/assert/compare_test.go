package assert //nolint:stdlib-test

import (
	"strings"
	"testing"
)

// ---- CheckGreater tests ----

func TestCheckGreater_IntGreaterPasses(t *testing.T) {
	result := CheckGreater(5, 3)
	if result != "" {
		t.Errorf("expected empty string for 5 > 3, got: %q", result)
	}
}

func TestCheckGreater_IntEqualFails(t *testing.T) {
	result := CheckGreater(4, 4)
	if result == "" {
		t.Fatal("expected non-empty error string for 4 > 4 (equal)")
	}
	if !strings.Contains(result, "Greater failed") {
		t.Errorf("error should contain 'Greater failed', got: %q", result)
	}
	if !strings.Contains(result, "not greater than") {
		t.Errorf("error should contain 'not greater than', got: %q", result)
	}
}

func TestCheckGreater_IntLessFails(t *testing.T) {
	result := CheckGreater(2, 7)
	if result == "" {
		t.Fatal("expected non-empty error string for 2 > 7")
	}
	if !strings.Contains(result, "Greater failed") {
		t.Errorf("error should contain 'Greater failed', got: %q", result)
	}
}

func TestCheckGreater_FloatPasses(t *testing.T) {
	result := CheckGreater(3.14, 2.71)
	if result != "" {
		t.Errorf("expected empty string for 3.14 > 2.71, got: %q", result)
	}
}

func TestCheckGreater_StringPasses(t *testing.T) {
	result := CheckGreater("b", "a")
	if result != "" {
		t.Errorf("expected empty string for \"b\" > \"a\", got: %q", result)
	}
}

// ---- CheckLess tests ----

func TestCheckLess_IntLessPasses(t *testing.T) {
	result := CheckLess(1, 10)
	if result != "" {
		t.Errorf("expected empty string for 1 < 10, got: %q", result)
	}
}

func TestCheckLess_IntEqualFails(t *testing.T) {
	result := CheckLess(5, 5)
	if result == "" {
		t.Fatal("expected non-empty error string for 5 < 5 (equal)")
	}
	if !strings.Contains(result, "Less failed") {
		t.Errorf("error should contain 'Less failed', got: %q", result)
	}
	if !strings.Contains(result, "not less than") {
		t.Errorf("error should contain 'not less than', got: %q", result)
	}
}

func TestCheckLess_IntGreaterFails(t *testing.T) {
	result := CheckLess(9, 3)
	if result == "" {
		t.Fatal("expected non-empty error string for 9 < 3")
	}
	if !strings.Contains(result, "Less failed") {
		t.Errorf("error should contain 'Less failed', got: %q", result)
	}
}

// ---- CheckGreaterOrEqual tests ----

func TestCheckGreaterOrEqual_GreaterPasses(t *testing.T) {
	result := CheckGreaterOrEqual(10, 5)
	if result != "" {
		t.Errorf("expected empty string for 10 >= 5, got: %q", result)
	}
}

func TestCheckGreaterOrEqual_EqualPasses(t *testing.T) {
	result := CheckGreaterOrEqual(7, 7)
	if result != "" {
		t.Errorf("expected empty string for 7 >= 7, got: %q", result)
	}
}

func TestCheckGreaterOrEqual_LessFails(t *testing.T) {
	result := CheckGreaterOrEqual(3, 8)
	if result == "" {
		t.Fatal("expected non-empty error string for 3 >= 8")
	}
	if !strings.Contains(result, "GreaterOrEqual failed") {
		t.Errorf("error should contain 'GreaterOrEqual failed', got: %q", result)
	}
	if !strings.Contains(result, "not greater than or equal to") {
		t.Errorf("error should contain 'not greater than or equal to', got: %q", result)
	}
}

// ---- CheckLessOrEqual tests ----

func TestCheckLessOrEqual_LessPasses(t *testing.T) {
	result := CheckLessOrEqual(2, 9)
	if result != "" {
		t.Errorf("expected empty string for 2 <= 9, got: %q", result)
	}
}

func TestCheckLessOrEqual_EqualPasses(t *testing.T) {
	result := CheckLessOrEqual(6, 6)
	if result != "" {
		t.Errorf("expected empty string for 6 <= 6, got: %q", result)
	}
}

func TestCheckLessOrEqual_GreaterFails(t *testing.T) {
	result := CheckLessOrEqual(11, 4)
	if result == "" {
		t.Fatal("expected non-empty error string for 11 <= 4")
	}
	if !strings.Contains(result, "LessOrEqual failed") {
		t.Errorf("error should contain 'LessOrEqual failed', got: %q", result)
	}
	if !strings.Contains(result, "not less than or equal to") {
		t.Errorf("error should contain 'not less than or equal to', got: %q", result)
	}
}
