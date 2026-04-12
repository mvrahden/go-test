// Package examples demonstrates all capabilities of the gotest library.
//
// Run all examples:
//
//	go test ./examples/ -v
package examples

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/mvrahden/go-test/gotest"
)

// ---------------------------------------------------------------------------
// 1. Basic suite with lifecycle hooks
// ---------------------------------------------------------------------------

// TestLifecycleHooks demonstrates BeforeAll, AfterAll, BeforeEach, AfterEach
// and the order in which they execute.
func TestLifecycleHooks(t *testing.T) {
	gotest.Run(t, func(s *gotest.S) {
		// Shared state lives in closure scope — no struct needed.
		var log []string

		s.BeforeAll(func(t *gotest.T) {
			log = append(log, "BeforeAll")
		})

		s.AfterAll(func(t *gotest.T) {
			log = append(log, "AfterAll")
			// At this point the full log is:
			// [BeforeAll, BeforeEach, test-a, AfterEach, BeforeEach, test-b, AfterEach, AfterAll]
			t.Assert(len(log)).Equals(8)
			t.Assert(log[0]).Equals("BeforeAll")
			t.Assert(log[len(log)-1]).Equals("AfterAll")
		})

		s.BeforeEach(func(t *gotest.T) {
			log = append(log, "BeforeEach")
		})

		s.AfterEach(func(t *gotest.T) {
			log = append(log, "AfterEach")
		})

		s.Test("test-a", func(t *gotest.T) {
			log = append(log, "test-a")
			t.Assert(log).Equals([]string{"BeforeAll", "BeforeEach", "test-a"})
		})

		s.Test("test-b", func(t *gotest.T) {
			log = append(log, "test-b")
			t.Assert(log).Equals([]string{
				"BeforeAll",
				"BeforeEach", "test-a", "AfterEach",
				"BeforeEach", "test-b",
			})
		})
	})
}

// ---------------------------------------------------------------------------
// 2. Shared mutable state via closures
// ---------------------------------------------------------------------------

// TestSharedState shows how closure-scoped variables replace struct fields
// for sharing state between hooks and tests.
func TestSharedState(t *testing.T) {
	gotest.Run(t, func(s *gotest.S) {
		// This is the equivalent of struct fields in a traditional test suite.
		var (
			users  map[string]string
			nextID int
		)

		s.BeforeAll(func(t *gotest.T) {
			users = make(map[string]string)
		})

		s.BeforeEach(func(t *gotest.T) {
			// Reset to a known state before each test.
			for k := range users {
				delete(users, k)
			}
			nextID = 1
		})

		addUser := func(name string) string {
			id := fmt.Sprintf("user-%d", nextID)
			nextID++
			users[id] = name
			return id
		}

		s.Test("add a user", func(t *gotest.T) {
			id := addUser("Alice")
			t.Assert(id).Equals("user-1")
			t.Assert(users).HasLength(1)
		})

		s.Test("each test starts fresh", func(t *gotest.T) {
			// BeforeEach cleared the map, so it's empty again.
			t.Assert(users).IsEmpty()
			t.Assert(nextID).Equals(1)

			addUser("Bob")
			addUser("Carol")
			t.Assert(users).HasLength(2)
		})
	})
}

// ---------------------------------------------------------------------------
// 3. Nested Describe with hook inheritance
// ---------------------------------------------------------------------------

// TestNestedDescribe shows how Describe blocks create nested scopes.
// Child scopes inherit parent BeforeEach/AfterEach hooks.
func TestNestedDescribe(t *testing.T) {
	gotest.Run(t, func(s *gotest.S) {
		var role string

		s.BeforeEach(func(t *gotest.T) {
			role = "guest"
		})

		s.Test("default role is guest", func(t *gotest.T) {
			t.Assert(role).Equals("guest")
		})

		s.Describe("authenticated user", func(s *gotest.S) {
			s.BeforeEach(func(t *gotest.T) {
				// Runs AFTER parent BeforeEach, so role is already "guest".
				role = "user"
			})

			s.Test("role is user", func(t *gotest.T) {
				t.Assert(role).Equals("user")
			})

			s.Describe("with admin privileges", func(s *gotest.S) {
				s.BeforeEach(func(t *gotest.T) {
					role = "admin"
				})

				s.Test("role is admin", func(t *gotest.T) {
					t.Assert(role).Equals("admin")
				})
			})
		})

		s.Describe("API key auth", func(s *gotest.S) {
			s.BeforeEach(func(t *gotest.T) {
				role = "service"
			})

			s.Test("role is service", func(t *gotest.T) {
				// Parent BeforeEach set "guest", then this scope's BeforeEach set "service".
				t.Assert(role).Equals("service")
			})
		})
	})
}

// ---------------------------------------------------------------------------
// 4. Deep nesting — hook execution order
// ---------------------------------------------------------------------------

// TestDeepNesting verifies that hooks execute in the correct order
// across three levels of nesting.
func TestDeepNesting(t *testing.T) {
	gotest.Run(t, func(s *gotest.S) {
		var trace []string

		s.BeforeEach(func(t *gotest.T) { trace = append(trace, "L0-setup") })
		s.AfterEach(func(t *gotest.T) { trace = append(trace, "L0-teardown") })

		s.Describe("level 1", func(s *gotest.S) {
			s.BeforeEach(func(t *gotest.T) { trace = append(trace, "L1-setup") })
			s.AfterEach(func(t *gotest.T) { trace = append(trace, "L1-teardown") })

			s.Describe("level 2", func(s *gotest.S) {
				s.BeforeEach(func(t *gotest.T) { trace = append(trace, "L2-setup") })
				s.AfterEach(func(t *gotest.T) { trace = append(trace, "L2-teardown") })

				s.Test("deep test", func(t *gotest.T) {
					trace = append(trace, "test")

					// BeforeEach hooks run parent-first.
					t.Assert(trace).Equals([]string{
						"L0-setup", "L1-setup", "L2-setup", "test",
					})
				})
			})
		})

		s.AfterAll(func(t *gotest.T) {
			// AfterEach hooks unwind child-first.
			t.Assert(trace).Equals([]string{
				"L0-setup", "L1-setup", "L2-setup", "test",
				"L2-teardown", "L1-teardown", "L0-teardown",
			})
		})
	})
}

// ---------------------------------------------------------------------------
// 5. Focus and Exclude
// ---------------------------------------------------------------------------

// TestFocusWithFTest demonstrates that FTest runs only the focused test.
func TestFocusWithFTest(t *testing.T) {
	var ran []string
	gotest.Run(t, func(s *gotest.S) {
		s.Test("normal-a", func(t *gotest.T) { ran = append(ran, "a") })
		s.FTest("focused-b", func(t *gotest.T) { ran = append(ran, "b") })
		s.Test("normal-c", func(t *gotest.T) { ran = append(ran, "c") })
	})
	if len(ran) != 1 || ran[0] != "b" {
		t.Fatalf("only focused test should run, got %v", ran)
	}
}

// TestExcludeWithXTest demonstrates that XTest skips the excluded test.
func TestExcludeWithXTest(t *testing.T) {
	var ran []string
	gotest.Run(t, func(s *gotest.S) {
		s.Test("a", func(t *gotest.T) { ran = append(ran, "a") })
		s.XTest("b-excluded", func(t *gotest.T) { ran = append(ran, "b") })
		s.Test("c", func(t *gotest.T) { ran = append(ran, "c") })
	})
	if len(ran) != 2 || ran[0] != "a" || ran[1] != "c" {
		t.Fatalf("excluded test should be skipped, got %v", ran)
	}
}

// TestFocusDescribe demonstrates that FDescribe runs only the focused group.
func TestFocusDescribe(t *testing.T) {
	var ran []string
	gotest.Run(t, func(s *gotest.S) {
		s.Describe("group-a", func(s *gotest.S) {
			s.Test("a1", func(t *gotest.T) { ran = append(ran, "a1") })
		})
		s.FDescribe("group-b", func(s *gotest.S) {
			s.Test("b1", func(t *gotest.T) { ran = append(ran, "b1") })
			s.Test("b2", func(t *gotest.T) { ran = append(ran, "b2") })
		})
		s.Describe("group-c", func(s *gotest.S) {
			s.Test("c1", func(t *gotest.T) { ran = append(ran, "c1") })
		})
	})
	if len(ran) != 2 || ran[0] != "b1" || ran[1] != "b2" {
		t.Fatalf("only focused group should run, got %v", ran)
	}
}

// TestExcludeDescribe demonstrates that XDescribe skips the entire group.
func TestExcludeDescribe(t *testing.T) {
	var ran []string
	gotest.Run(t, func(s *gotest.S) {
		s.Describe("included", func(s *gotest.S) {
			s.Test("a", func(t *gotest.T) { ran = append(ran, "a") })
		})
		s.XDescribe("excluded", func(s *gotest.S) {
			s.Test("b", func(t *gotest.T) { ran = append(ran, "b") })
		})
	})
	if len(ran) != 1 || ran[0] != "a" {
		t.Fatalf("excluded group should be skipped, got %v", ran)
	}
}

// ---------------------------------------------------------------------------
// 6. Parallel tests
// ---------------------------------------------------------------------------

// TestParallelExecution demonstrates TestParallel for concurrent test execution.
func TestParallelExecution(t *testing.T) {
	gotest.Run(t, func(s *gotest.S) {
		var mu sync.Mutex
		results := map[string]int{}

		s.TestParallel("compute-a", func(t *gotest.T) {
			val := expensiveComputation("a")
			mu.Lock()
			results["a"] = val
			mu.Unlock()
			t.Assert(val).Equals(1)
		})

		s.TestParallel("compute-b", func(t *gotest.T) {
			val := expensiveComputation("b")
			mu.Lock()
			results["b"] = val
			mu.Unlock()
			t.Assert(val).Equals(2)
		})

		s.TestParallel("compute-c", func(t *gotest.T) {
			val := expensiveComputation("c")
			mu.Lock()
			results["c"] = val
			mu.Unlock()
			t.Assert(val).Equals(3)
		})
	})
}

func expensiveComputation(key string) int {
	return map[string]int{"a": 1, "b": 2, "c": 3}[key]
}

// ---------------------------------------------------------------------------
// 7. Parallel tests with BeforeEach hooks
// ---------------------------------------------------------------------------

// TestParallelWithHooks shows that BeforeEach runs before the parallel barrier,
// so setup completes synchronously before tests fan out.
func TestParallelWithHooks(t *testing.T) {
	gotest.Run(t, func(s *gotest.S) {
		var mu sync.Mutex
		setupCount := 0

		s.BeforeEach(func(t *gotest.T) {
			mu.Lock()
			setupCount++
			mu.Unlock()
		})

		s.TestParallel("p1", func(t *gotest.T) {})
		s.TestParallel("p2", func(t *gotest.T) {})
		s.TestParallel("p3", func(t *gotest.T) {})

		s.AfterAll(func(t *gotest.T) {
			t.Assert(setupCount).Equals(3)
		})
	})
}

// ---------------------------------------------------------------------------
// 8. Fluent assertions — comprehensive showcase
// ---------------------------------------------------------------------------

// TestAssertions demonstrates every assertion method available on AssertContext.
func TestAssertions(t *testing.T) {
	gotest.Run(t, func(s *gotest.S) {

		s.Describe("boolean", func(s *gotest.S) {
			s.Test("IsTrue", func(t *gotest.T) {
				t.Assert(true).IsTrue()
				t.Assert(2 > 1).IsTrue()
			})

			s.Test("IsFalse", func(t *gotest.T) {
				t.Assert(false).IsFalse()
				t.Assert(1 > 2).IsFalse()
			})
		})

		s.Describe("equality", func(s *gotest.S) {
			s.Test("primitives", func(t *gotest.T) {
				t.Assert(42).Equals(42)
				t.Assert(3.14).Equals(3.14)
				t.Assert("hello").Equals("hello")
			})

			s.Test("slices", func(t *gotest.T) {
				t.Assert([]int{1, 2, 3}).Equals([]int{1, 2, 3})
				t.Assert([]string{"a", "b"}).Equals([]string{"a", "b"})
			})

			s.Test("maps", func(t *gotest.T) {
				t.Assert(map[string]int{"a": 1}).Equals(map[string]int{"a": 1})
			})

			s.Test("structs", func(t *gotest.T) {
				type Point struct{ X, Y int }
				t.Assert(Point{1, 2}).Equals(Point{1, 2})
			})
		})

		s.Describe("nil checks", func(s *gotest.S) {
			s.Test("IsNil", func(t *gotest.T) {
				t.Assert(nil).IsNil()

				var p *int
				t.Assert(p).IsNil()

				var err error
				t.Assert(err).IsNil()

				var s []int
				t.Assert(s).IsNil()
			})

			s.Test("IsNotNil", func(t *gotest.T) {
				t.Assert(42).IsNotNil()
				t.Assert("").IsNotNil()
				t.Assert([]int{}).IsNotNil()
				t.Assert(fmt.Errorf("oops")).IsNotNil()
			})
		})

		s.Describe("zero value", func(s *gotest.S) {
			s.Test("IsZero", func(t *gotest.T) {
				t.Assert(0).IsZero()
				t.Assert("").IsZero()
				t.Assert(false).IsZero()
				t.Assert(0.0).IsZero()

				type Config struct{ Port int }
				t.Assert(Config{}).IsZero()
			})
		})

		s.Describe("length and emptiness", func(s *gotest.S) {
			s.Test("HasLength on slices", func(t *gotest.T) {
				t.Assert([]int{1, 2, 3}).HasLength(3)
				t.Assert([]int{}).HasLength(0)
			})

			s.Test("HasLength on strings", func(t *gotest.T) {
				t.Assert("hello").HasLength(5)
				t.Assert("").HasLength(0)
			})

			s.Test("HasLength on maps", func(t *gotest.T) {
				t.Assert(map[string]int{"a": 1, "b": 2}).HasLength(2)
			})

			s.Test("IsEmpty", func(t *gotest.T) {
				t.Assert([]int{}).IsEmpty()
				t.Assert("").IsEmpty()
				t.Assert(map[string]int{}).IsEmpty()
			})
		})

		s.Describe("containment", func(s *gotest.S) {
			s.Test("slice contains element", func(t *gotest.T) {
				t.Assert([]int{10, 20, 30}).Contains(20)
				t.Assert([]string{"go", "rust", "zig"}).Contains("rust")
			})

			s.Test("string contains substring", func(t *gotest.T) {
				t.Assert("hello world").Contains("world")
				t.Assert("foo-bar-baz").Contains("-bar-")
			})
		})
	})
}

// ---------------------------------------------------------------------------
// 9. Mixing with stdlib testing
// ---------------------------------------------------------------------------

// TestMixedWithStdlib shows gotest suites coexisting with regular Go tests
// in the same file. This is a standard test function — no gotest involved.
func TestMixedWithStdlib(t *testing.T) {
	// Regular table-driven test.
	cases := []struct {
		input    string
		expected string
	}{
		{"hello", "HELLO"},
		{"world", "WORLD"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := strings.ToUpper(tc.input)
			if got != tc.expected {
				t.Fatalf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}

// TestStdlibAlongsideSuite shows a gotest suite right next to the stdlib test above.
func TestStdlibAlongsideSuite(t *testing.T) {
	gotest.Run(t, func(s *gotest.S) {
		s.Test("strings.ToUpper via suite", func(t *gotest.T) {
			t.Assert(strings.ToUpper("hello")).Equals("HELLO")
		})
	})
}

// ---------------------------------------------------------------------------
// 10. It() for BDD-style sub-descriptions
// ---------------------------------------------------------------------------

// TestItSubtests demonstrates the It() method on T for BDD-style nesting
// within a single test case.
func TestItSubtests(t *testing.T) {
	gotest.Run(t, func(s *gotest.S) {
		s.Test("sorting algorithms", func(t *gotest.T) {
			input := []int{3, 1, 4, 1, 5, 9, 2, 6}

			t.It("sorts ascending", func(t *gotest.T) {
				data := make([]int, len(input))
				copy(data, input)
				sort.Ints(data)
				t.Assert(data).Equals([]int{1, 1, 2, 3, 4, 5, 6, 9})
			})

			t.It("sorts descending", func(t *gotest.T) {
				data := make([]int, len(input))
				copy(data, input)
				sort.Sort(sort.Reverse(sort.IntSlice(data)))
				t.Assert(data).Equals([]int{9, 6, 5, 4, 3, 2, 1, 1})
			})
		})
	})
}

// ---------------------------------------------------------------------------
// 11. Accessing the underlying *testing.T
// ---------------------------------------------------------------------------

// TestAccessUnderlyingT shows how to use t.T() to access stdlib methods
// like Skip, Helper, or Logf.
func TestAccessUnderlyingT(t *testing.T) {
	gotest.Run(t, func(s *gotest.S) {
		s.Test("log and skip", func(t *gotest.T) {
			t.T().Log("this message appears with -v")

			if testing.Short() {
				t.T().Skip("skipping in short mode")
			}
		})

		s.Test("test name", func(t *gotest.T) {
			name := t.T().Name()
			t.Assert(strings.Contains(name, "test_name")).IsTrue()
		})
	})
}

// ---------------------------------------------------------------------------
// 12. Real-world pattern: in-memory repository
// ---------------------------------------------------------------------------

// A minimal in-memory repository to demonstrate a realistic test pattern.
type UserRepo struct {
	users map[string]string
}

func NewUserRepo() *UserRepo              { return &UserRepo{users: map[string]string{}} }
func (r *UserRepo) Add(id, name string)   { r.users[id] = name }
func (r *UserRepo) Get(id string) string  { return r.users[id] }
func (r *UserRepo) Count() int            { return len(r.users) }
func (r *UserRepo) Delete(id string)      { delete(r.users, id) }
func (r *UserRepo) Has(id string) bool    { _, ok := r.users[id]; return ok }

func TestUserRepository(t *testing.T) {
	gotest.Run(t, func(s *gotest.S) {
		var repo *UserRepo

		s.BeforeEach(func(t *gotest.T) {
			repo = NewUserRepo()
		})

		s.Test("starts empty", func(t *gotest.T) {
			t.Assert(repo.Count()).Equals(0)
		})

		s.Test("add and retrieve", func(t *gotest.T) {
			repo.Add("1", "Alice")
			t.Assert(repo.Get("1")).Equals("Alice")
			t.Assert(repo.Count()).Equals(1)
		})

		s.Describe("with seeded data", func(s *gotest.S) {
			s.BeforeEach(func(t *gotest.T) {
				repo.Add("1", "Alice")
				repo.Add("2", "Bob")
			})

			s.Test("has two users", func(t *gotest.T) {
				t.Assert(repo.Count()).Equals(2)
			})

			s.Test("delete removes user", func(t *gotest.T) {
				repo.Delete("1")
				t.Assert(repo.Has("1")).IsFalse()
				t.Assert(repo.Count()).Equals(1)
			})

			s.Test("get returns correct user", func(t *gotest.T) {
				t.Assert(repo.Get("2")).Equals("Bob")
			})

			s.Describe("after clearing all", func(s *gotest.S) {
				s.BeforeEach(func(t *gotest.T) {
					repo.Delete("1")
					repo.Delete("2")
				})

				s.Test("repo is empty again", func(t *gotest.T) {
					t.Assert(repo.Count()).Equals(0)
				})
			})
		})
	})
}
