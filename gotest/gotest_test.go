package gotest_test

import (
	"testing"

	"github.com/mvrahden/go-test/gotest"
)

func TestRun_executes_registered_tests(t *testing.T) {
	var ran bool
	gotest.Run(t, func(s *gotest.S) {
		s.Test("basic", func(t *gotest.T) {
			ran = true
		})
	})
	if !ran {
		t.Fatal("test should have executed")
	}
}

func TestRun_test_name_becomes_subtest(t *testing.T) {
	gotest.Run(t, func(s *gotest.S) {
		s.Test("my_test", func(tt *gotest.T) {
			expected := t.Name() + "/my_test"
			if tt.T().Name() != expected {
				t.Fatalf("expected subtest name %q, got %q", expected, tt.T().Name())
			}
		})
	})
}

func TestRun_multiple_tests_execute_in_order(t *testing.T) {
	var order []string
	gotest.Run(t, func(s *gotest.S) {
		s.Test("first", func(t *gotest.T) {
			order = append(order, "first")
		})
		s.Test("second", func(t *gotest.T) {
			order = append(order, "second")
		})
		s.Test("third", func(t *gotest.T) {
			order = append(order, "third")
		})
	})
	if len(order) != 3 || order[0] != "first" || order[1] != "second" || order[2] != "third" {
		t.Fatalf("expected [first second third], got %v", order)
	}
}

func TestRun_BeforeAll_runs_once_before_all_tests(t *testing.T) {
	var order []string
	gotest.Run(t, func(s *gotest.S) {
		s.BeforeAll(func(t *gotest.T) {
			order = append(order, "beforeAll")
		})
		s.Test("a", func(t *gotest.T) {
			order = append(order, "testA")
		})
		s.Test("b", func(t *gotest.T) {
			order = append(order, "testB")
		})
	})
	expected := []string{"beforeAll", "testA", "testB"}
	if !slicesEqual(order, expected) {
		t.Fatalf("expected %v, got %v", expected, order)
	}
}

func TestRun_AfterAll_runs_once_after_all_tests(t *testing.T) {
	var order []string
	gotest.Run(t, func(s *gotest.S) {
		s.AfterAll(func(t *gotest.T) {
			order = append(order, "afterAll")
		})
		s.Test("a", func(t *gotest.T) {
			order = append(order, "testA")
		})
		s.Test("b", func(t *gotest.T) {
			order = append(order, "testB")
		})
	})
	expected := []string{"testA", "testB", "afterAll"}
	if !slicesEqual(order, expected) {
		t.Fatalf("expected %v, got %v", expected, order)
	}
}

func TestRun_BeforeAll_and_AfterAll_bracket_tests(t *testing.T) {
	var order []string
	gotest.Run(t, func(s *gotest.S) {
		s.BeforeAll(func(t *gotest.T) {
			order = append(order, "beforeAll")
		})
		s.AfterAll(func(t *gotest.T) {
			order = append(order, "afterAll")
		})
		s.Test("a", func(t *gotest.T) {
			order = append(order, "testA")
		})
	})
	expected := []string{"beforeAll", "testA", "afterAll"}
	if !slicesEqual(order, expected) {
		t.Fatalf("expected %v, got %v", expected, order)
	}
}

func TestRun_no_tests_still_runs_hooks(t *testing.T) {
	var order []string
	gotest.Run(t, func(s *gotest.S) {
		s.BeforeAll(func(t *gotest.T) {
			order = append(order, "beforeAll")
		})
		s.AfterAll(func(t *gotest.T) {
			order = append(order, "afterAll")
		})
	})
	expected := []string{"beforeAll", "afterAll"}
	if !slicesEqual(order, expected) {
		t.Fatalf("expected %v, got %v", expected, order)
	}
}

// slicesEqual is a test helper — compares two string slices.
func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
