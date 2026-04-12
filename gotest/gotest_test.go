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

func TestRun_BeforeEach_runs_before_each_test(t *testing.T) {
	var order []string
	gotest.Run(t, func(s *gotest.S) {
		s.BeforeEach(func(t *gotest.T) {
			order = append(order, "beforeEach")
		})
		s.Test("a", func(t *gotest.T) {
			order = append(order, "testA")
		})
		s.Test("b", func(t *gotest.T) {
			order = append(order, "testB")
		})
	})
	expected := []string{"beforeEach", "testA", "beforeEach", "testB"}
	if !slicesEqual(order, expected) {
		t.Fatalf("expected %v, got %v", expected, order)
	}
}

func TestRun_AfterEach_runs_after_each_test(t *testing.T) {
	var order []string
	gotest.Run(t, func(s *gotest.S) {
		s.AfterEach(func(t *gotest.T) {
			order = append(order, "afterEach")
		})
		s.Test("a", func(t *gotest.T) {
			order = append(order, "testA")
		})
		s.Test("b", func(t *gotest.T) {
			order = append(order, "testB")
		})
	})
	expected := []string{"testA", "afterEach", "testB", "afterEach"}
	if !slicesEqual(order, expected) {
		t.Fatalf("expected %v, got %v", expected, order)
	}
}

func TestRun_full_lifecycle_ordering(t *testing.T) {
	var order []string
	gotest.Run(t, func(s *gotest.S) {
		s.BeforeAll(func(t *gotest.T) { order = append(order, "beforeAll") })
		s.AfterAll(func(t *gotest.T) { order = append(order, "afterAll") })
		s.BeforeEach(func(t *gotest.T) { order = append(order, "beforeEach") })
		s.AfterEach(func(t *gotest.T) { order = append(order, "afterEach") })
		s.Test("a", func(t *gotest.T) { order = append(order, "testA") })
		s.Test("b", func(t *gotest.T) { order = append(order, "testB") })
	})
	expected := []string{
		"beforeAll",
		"beforeEach", "testA", "afterEach",
		"beforeEach", "testB", "afterEach",
		"afterAll",
	}
	if !slicesEqual(order, expected) {
		t.Fatalf("expected %v, got %v", expected, order)
	}
}

func TestRun_AfterEach_runs_even_when_test_fails(t *testing.T) {
	var afterEachRan bool
	gotest.Run(t, func(s *gotest.S) {
		s.AfterEach(func(t *gotest.T) {
			afterEachRan = true
		})
		s.Test("fails", func(t *gotest.T) {
			t.T().Fail()
		})
	})
	if !afterEachRan {
		t.Fatal("AfterEach should run even when test fails")
	}
}

func TestRun_Describe_creates_nested_subtest(t *testing.T) {
	var ran bool
	gotest.Run(t, func(s *gotest.S) {
		s.Describe("group", func(s *gotest.S) {
			s.Test("inner", func(t *gotest.T) {
				ran = true
			})
		})
	})
	if !ran {
		t.Fatal("nested test should have executed")
	}
}

func TestRun_Describe_inherits_parent_BeforeEach(t *testing.T) {
	var order []string
	gotest.Run(t, func(s *gotest.S) {
		s.BeforeEach(func(t *gotest.T) {
			order = append(order, "parentBeforeEach")
		})
		s.Describe("child", func(s *gotest.S) {
			s.BeforeEach(func(t *gotest.T) {
				order = append(order, "childBeforeEach")
			})
			s.Test("inner", func(t *gotest.T) {
				order = append(order, "test")
			})
		})
	})
	expected := []string{"parentBeforeEach", "childBeforeEach", "test"}
	if !slicesEqual(order, expected) {
		t.Fatalf("expected %v, got %v", expected, order)
	}
}

func TestRun_Describe_AfterEach_unwinds_in_reverse(t *testing.T) {
	var order []string
	gotest.Run(t, func(s *gotest.S) {
		s.AfterEach(func(t *gotest.T) {
			order = append(order, "parentAfterEach")
		})
		s.Describe("child", func(s *gotest.S) {
			s.AfterEach(func(t *gotest.T) {
				order = append(order, "childAfterEach")
			})
			s.Test("inner", func(t *gotest.T) {
				order = append(order, "test")
			})
		})
	})
	expected := []string{"test", "childAfterEach", "parentAfterEach"}
	if !slicesEqual(order, expected) {
		t.Fatalf("expected %v, got %v", expected, order)
	}
}

func TestRun_Describe_full_nested_lifecycle(t *testing.T) {
	var order []string
	gotest.Run(t, func(s *gotest.S) {
		s.BeforeAll(func(t *gotest.T) { order = append(order, "parentBeforeAll") })
		s.AfterAll(func(t *gotest.T) { order = append(order, "parentAfterAll") })
		s.BeforeEach(func(t *gotest.T) { order = append(order, "parentBE") })
		s.AfterEach(func(t *gotest.T) { order = append(order, "parentAE") })

		s.Test("top", func(t *gotest.T) { order = append(order, "topTest") })

		s.Describe("child", func(s *gotest.S) {
			s.BeforeAll(func(t *gotest.T) { order = append(order, "childBeforeAll") })
			s.AfterAll(func(t *gotest.T) { order = append(order, "childAfterAll") })
			s.BeforeEach(func(t *gotest.T) { order = append(order, "childBE") })
			s.AfterEach(func(t *gotest.T) { order = append(order, "childAE") })

			s.Test("nested", func(t *gotest.T) { order = append(order, "nestedTest") })
		})
	})
	expected := []string{
		"parentBeforeAll",
		"parentBE", "topTest", "parentAE",
		"childBeforeAll",
		"parentBE", "childBE", "nestedTest", "childAE", "parentAE",
		"childAfterAll",
		"parentAfterAll",
	}
	if !slicesEqual(order, expected) {
		t.Fatalf("expected:\n  %v\ngot:\n  %v", expected, order)
	}
}

func TestRun_Describe_double_nesting(t *testing.T) {
	var order []string
	gotest.Run(t, func(s *gotest.S) {
		s.BeforeEach(func(t *gotest.T) { order = append(order, "L0-BE") })
		s.Describe("L1", func(s *gotest.S) {
			s.BeforeEach(func(t *gotest.T) { order = append(order, "L1-BE") })
			s.Describe("L2", func(s *gotest.S) {
				s.BeforeEach(func(t *gotest.T) { order = append(order, "L2-BE") })
				s.Test("deep", func(t *gotest.T) { order = append(order, "test") })
			})
		})
	})
	expected := []string{"L0-BE", "L1-BE", "L2-BE", "test"}
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
