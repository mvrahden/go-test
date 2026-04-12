package gotest_test

import (
	"bytes"
	"testing"

	"github.com/mvrahden/go-test/gotest"
)

func TestExample_BasicSuite(t *testing.T) {
	gotest.Run(t, func(s *gotest.S) {
		var buf bytes.Buffer

		s.BeforeAll(func(t *gotest.T) {
			buf.WriteString("setup;")
		})

		s.AfterAll(func(t *gotest.T) {
			buf.WriteString("teardown;")
		})

		s.BeforeEach(func(t *gotest.T) {
			buf.WriteString("beforeEach;")
		})

		s.AfterEach(func(t *gotest.T) {
			buf.WriteString("afterEach;")
		})

		s.Test("test1", func(t *gotest.T) {
			buf.WriteString("test1;")
		})

		s.Test("test2", func(t *gotest.T) {
			buf.WriteString("test2;")
		})
	})
}

func TestExample_NestedDescribe(t *testing.T) {
	gotest.Run(t, func(s *gotest.S) {
		var level string

		s.BeforeEach(func(t *gotest.T) {
			level = "base"
		})

		s.Test("uses base level", func(t *gotest.T) {
			t.Assert(level).Equals("base")
		})

		s.Describe("with premium", func(s *gotest.S) {
			s.BeforeEach(func(t *gotest.T) {
				level = level + "+premium"
			})

			s.Test("has premium", func(t *gotest.T) {
				t.Assert(level).Equals("base+premium")
			})

			s.Describe("during sale", func(s *gotest.S) {
				s.BeforeEach(func(t *gotest.T) {
					level = level + "+sale"
				})

				s.Test("has all modifiers", func(t *gotest.T) {
					t.Assert(level).Equals("base+premium+sale")
				})
			})
		})
	})
}

func TestExample_FocusAndExclude(t *testing.T) {
	var executed []string
	gotest.Run(t, func(s *gotest.S) {
		s.Test("runs", func(t *gotest.T) {
			executed = append(executed, "runs")
		})
		s.XTest("skipped", func(t *gotest.T) {
			executed = append(executed, "skipped")
		})
	})

	if len(executed) != 1 || executed[0] != "runs" {
		t.Fatalf("expected only 'runs' to execute, got %v", executed)
	}
}

func TestExample_Assertions(t *testing.T) {
	gotest.Run(t, func(s *gotest.S) {
		s.Test("boolean assertions", func(t *gotest.T) {
			t.Assert(true).IsTrue()
			t.Assert(false).IsFalse()
		})

		s.Test("equality", func(t *gotest.T) {
			t.Assert(42).Equals(42)
			t.Assert("hello").Equals("hello")
			t.Assert([]int{1, 2, 3}).Equals([]int{1, 2, 3})
		})

		s.Test("nil checks", func(t *gotest.T) {
			t.Assert(nil).IsNil()
			t.Assert(42).IsNotNil()
		})

		s.Test("collections", func(t *gotest.T) {
			t.Assert([]int{1, 2, 3}).HasLength(3)
			t.Assert([]int{}).IsEmpty()
			t.Assert([]int{1, 2, 3}).Contains(2)
			t.Assert("hello world").Contains("world")
		})
	})
}

func TestExample_Parallel(t *testing.T) {
	gotest.Run(t, func(s *gotest.S) {
		s.TestParallel("p1", func(t *gotest.T) {
			t.Assert(1 + 1).Equals(2)
		})
		s.TestParallel("p2", func(t *gotest.T) {
			t.Assert(2 + 2).Equals(4)
		})
	})
}
