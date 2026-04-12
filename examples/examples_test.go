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

func TestLifecycleHooks(t *testing.T) {
	gotest.Run(t, func(s *gotest.S) {
		var log []string

		s.BeforeAll(func(t *gotest.T) {
			log = append(log, "BeforeAll")
		})

		s.AfterAll(func(t *gotest.T) {
			log = append(log, "AfterAll")
			gotest.Equal(t, 8, len(log))
			gotest.Equal(t, "BeforeAll", log[0])
			gotest.Equal(t, "AfterAll", log[len(log)-1])
		})

		s.BeforeEach(func(t *gotest.T) { log = append(log, "BeforeEach") })
		s.AfterEach(func(t *gotest.T) { log = append(log, "AfterEach") })

		s.Test("test-a", func(t *gotest.T) {
			log = append(log, "test-a")
			gotest.Equal(t, []string{"BeforeAll", "BeforeEach", "test-a"}, log)
		})

		s.Test("test-b", func(t *gotest.T) {
			log = append(log, "test-b")
			gotest.Equal(t, []string{
				"BeforeAll",
				"BeforeEach", "test-a", "AfterEach",
				"BeforeEach", "test-b",
			}, log)
		})
	})
}

// ---------------------------------------------------------------------------
// 2. Shared mutable state via closures
// ---------------------------------------------------------------------------

func TestSharedState(t *testing.T) {
	gotest.Run(t, func(s *gotest.S) {
		var (
			users  map[string]string
			nextID int
		)

		s.BeforeAll(func(t *gotest.T) {
			users = make(map[string]string)
		})

		s.BeforeEach(func(t *gotest.T) {
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
			gotest.Equal(t, "user-1", id)
			gotest.Len(t, users, 1)
		})

		s.Test("each test starts fresh", func(t *gotest.T) {
			gotest.Empty(t, users)
			gotest.Equal(t, 1, nextID)

			addUser("Bob")
			addUser("Carol")
			gotest.Len(t, users, 2)
		})
	})
}

// ---------------------------------------------------------------------------
// 3. Nested Describe with hook inheritance
// ---------------------------------------------------------------------------

func TestNestedDescribe(t *testing.T) {
	gotest.Run(t, func(s *gotest.S) {
		var role string

		s.BeforeEach(func(t *gotest.T) { role = "guest" })

		s.Test("default role is guest", func(t *gotest.T) {
			gotest.Equal(t, "guest", role)
		})

		s.Describe("authenticated user", func(s *gotest.S) {
			s.BeforeEach(func(t *gotest.T) { role = "user" })

			s.Test("role is user", func(t *gotest.T) {
				gotest.Equal(t, "user", role)
			})

			s.Describe("with admin privileges", func(s *gotest.S) {
				s.BeforeEach(func(t *gotest.T) { role = "admin" })

				s.Test("role is admin", func(t *gotest.T) {
					gotest.Equal(t, "admin", role)
				})
			})
		})

		s.Describe("API key auth", func(s *gotest.S) {
			s.BeforeEach(func(t *gotest.T) { role = "service" })

			s.Test("role is service", func(t *gotest.T) {
				gotest.Equal(t, "service", role)
			})
		})
	})
}

// ---------------------------------------------------------------------------
// 4. Deep nesting — hook execution order
// ---------------------------------------------------------------------------

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
					gotest.Equal(t, []string{
						"L0-setup", "L1-setup", "L2-setup", "test",
					}, trace)
				})
			})
		})

		s.AfterAll(func(t *gotest.T) {
			gotest.Equal(t, []string{
				"L0-setup", "L1-setup", "L2-setup", "test",
				"L2-teardown", "L1-teardown", "L0-teardown",
			}, trace)
		})
	})
}

// ---------------------------------------------------------------------------
// 5. Focus and Exclude
// ---------------------------------------------------------------------------

func TestFocusWithFTest(t *testing.T) {
	var ran []string
	gotest.Run(t, func(s *gotest.S) {
		s.Test("normal-a", func(t *gotest.T) { ran = append(ran, "a") })
		s.FTest("focused-b", func(t *gotest.T) { ran = append(ran, "b") })
		s.Test("normal-c", func(t *gotest.T) { ran = append(ran, "c") })
	})
	gotest.Equal(t, []string{"b"}, ran)
}

func TestExcludeWithXTest(t *testing.T) {
	var ran []string
	gotest.Run(t, func(s *gotest.S) {
		s.Test("a", func(t *gotest.T) { ran = append(ran, "a") })
		s.XTest("b-excluded", func(t *gotest.T) { ran = append(ran, "b") })
		s.Test("c", func(t *gotest.T) { ran = append(ran, "c") })
	})
	gotest.Equal(t, []string{"a", "c"}, ran)
}

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
	gotest.Equal(t, []string{"b1", "b2"}, ran)
}

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
	gotest.Equal(t, []string{"a"}, ran)
}

// ---------------------------------------------------------------------------
// 6. Parallel tests
// ---------------------------------------------------------------------------

func TestParallelExecution(t *testing.T) {
	gotest.Run(t, func(s *gotest.S) {
		var mu sync.Mutex
		results := map[string]int{}

		s.TestParallel("compute-a", func(t *gotest.T) {
			val := expensiveComputation("a")
			mu.Lock()
			results["a"] = val
			mu.Unlock()
			gotest.Equal(t, 1, val)
		})

		s.TestParallel("compute-b", func(t *gotest.T) {
			val := expensiveComputation("b")
			mu.Lock()
			results["b"] = val
			mu.Unlock()
			gotest.Equal(t, 2, val)
		})

		s.TestParallel("compute-c", func(t *gotest.T) {
			val := expensiveComputation("c")
			mu.Lock()
			results["c"] = val
			mu.Unlock()
			gotest.Equal(t, 3, val)
		})
	})
}

func expensiveComputation(key string) int {
	return map[string]int{"a": 1, "b": 2, "c": 3}[key]
}

// ---------------------------------------------------------------------------
// 7. Parallel tests with BeforeEach hooks
// ---------------------------------------------------------------------------

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
			gotest.Equal(t, 3, setupCount)
		})
	})
}

// ---------------------------------------------------------------------------
// 8. Assertions — comprehensive showcase
// ---------------------------------------------------------------------------

func TestAssertions(t *testing.T) {
	gotest.Run(t, func(s *gotest.S) {

		s.Describe("boolean", func(s *gotest.S) {
			s.Test("True and False", func(t *gotest.T) {
				gotest.True(t, 2 > 1)
				gotest.False(t, 1 > 2)
			})
		})

		s.Describe("equality", func(s *gotest.S) {
			s.Test("primitives", func(t *gotest.T) {
				gotest.Equal(t, 42, 42)
				gotest.Equal(t, 3.14, 3.14)
				gotest.Equal(t, "hello", "hello")
			})

			s.Test("slices", func(t *gotest.T) {
				gotest.Equal(t, []int{1, 2, 3}, []int{1, 2, 3})
			})

			s.Test("maps", func(t *gotest.T) {
				gotest.Equal(t, map[string]int{"a": 1}, map[string]int{"a": 1})
			})

			s.Test("structs", func(t *gotest.T) {
				type Point struct{ X, Y int }
				gotest.Equal(t, Point{1, 2}, Point{1, 2})
			})
		})

		s.Describe("error handling", func(s *gotest.S) {
			s.Test("NoError and Error", func(t *gotest.T) {
				gotest.NoError(t, nil)
				gotest.Error(t, fmt.Errorf("oops"))
			})

			s.Test("ErrorContains", func(t *gotest.T) {
				gotest.ErrorContains(t, fmt.Errorf("not found: user 42"), "not found")
			})
		})

		s.Describe("zero value", func(s *gotest.S) {
			s.Test("Zero and NotZero", func(t *gotest.T) {
				gotest.Zero(t, 0)
				gotest.Zero(t, "")
				gotest.NotZero(t, 1)
				gotest.NotZero(t, "x")
			})
		})

		s.Describe("collections", func(s *gotest.S) {
			s.Test("Len and Empty", func(t *gotest.T) {
				gotest.Len(t, []int{1, 2, 3}, 3)
				gotest.Empty(t, []int{})
				gotest.NotEmpty(t, []int{1})
			})

			s.Test("Contains", func(t *gotest.T) {
				gotest.Contains(t, []string{"go", "rust", "zig"}, "rust")
				gotest.Contains(t, "hello world", "world")
				gotest.NotContains(t, []int{1, 2}, 99)
			})

			s.Test("ElementsMatch", func(t *gotest.T) {
				gotest.ElementsMatch(t, []int{3, 1, 2}, []int{1, 2, 3})
			})

			s.Test("Subset", func(t *gotest.T) {
				gotest.Subset(t, []int{1, 2, 3, 4}, []int{2, 4})
			})
		})

		s.Describe("comparison", func(s *gotest.S) {
			s.Test("Greater and Less", func(t *gotest.T) {
				gotest.Greater(t, 2, 1)
				gotest.Less(t, 1, 2)
				gotest.GreaterOrEqual(t, 2, 2)
				gotest.LessOrEqual(t, 1, 2)
			})
		})

		s.Describe("fluent API", func(s *gotest.S) {
			s.Test("Assert chain", func(t *gotest.T) {
				t.Assert(true).IsTrue()
				t.Assert(false).IsFalse()
				t.Assert(42).Equal(42)
				t.Assert([]int{1, 2, 3}).HasLength(3)
				t.Assert([]int{}).Empty()
				t.Assert([]int{1, 2, 3}).Contains(2)
				t.Assert("hello world").Contains("world")
			})
		})
	})
}

// ---------------------------------------------------------------------------
// 9. Mixing with stdlib testing
// ---------------------------------------------------------------------------

func TestMixedWithStdlib(t *testing.T) {
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

func TestStdlibAlongsideSuite(t *testing.T) {
	gotest.Run(t, func(s *gotest.S) {
		s.Test("strings.ToUpper via suite", func(t *gotest.T) {
			gotest.Equal(t, "HELLO", strings.ToUpper("hello"))
		})
	})
}

// ---------------------------------------------------------------------------
// 10. It() for BDD-style sub-descriptions
// ---------------------------------------------------------------------------

func TestItSubtests(t *testing.T) {
	gotest.Run(t, func(s *gotest.S) {
		s.Test("sorting algorithms", func(t *gotest.T) {
			input := []int{3, 1, 4, 1, 5, 9, 2, 6}

			t.It("sorts ascending", func(t *gotest.T) {
				data := make([]int, len(input))
				copy(data, input)
				sort.Ints(data)
				gotest.Equal(t, []int{1, 1, 2, 3, 4, 5, 6, 9}, data)
			})

			t.It("sorts descending", func(t *gotest.T) {
				data := make([]int, len(input))
				copy(data, input)
				sort.Sort(sort.Reverse(sort.IntSlice(data)))
				gotest.Equal(t, []int{9, 6, 5, 4, 3, 2, 1, 1}, data)
			})
		})
	})
}

// ---------------------------------------------------------------------------
// 11. Accessing the underlying *testing.T
// ---------------------------------------------------------------------------

func TestAccessUnderlyingT(t *testing.T) {
	gotest.Run(t, func(s *gotest.S) {
		s.Test("log and skip", func(t *gotest.T) {
			t.T().Log("this message appears with -v")

			if testing.Short() {
				t.T().Skip("skipping in short mode")
			}
		})

		s.Test("test name", func(t *gotest.T) {
			gotest.True(t, strings.Contains(t.T().Name(), "test_name"))
		})
	})
}

// ---------------------------------------------------------------------------
// 12. Real-world pattern: in-memory repository
// ---------------------------------------------------------------------------

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
			gotest.Equal(t, 0, repo.Count())
		})

		s.Test("add and retrieve", func(t *gotest.T) {
			repo.Add("1", "Alice")
			gotest.Equal(t, "Alice", repo.Get("1"))
			gotest.Equal(t, 1, repo.Count())
		})

		s.Describe("with seeded data", func(s *gotest.S) {
			s.BeforeEach(func(t *gotest.T) {
				repo.Add("1", "Alice")
				repo.Add("2", "Bob")
			})

			s.Test("has two users", func(t *gotest.T) {
				gotest.Equal(t, 2, repo.Count())
			})

			s.Test("delete removes user", func(t *gotest.T) {
				repo.Delete("1")
				gotest.False(t, repo.Has("1"))
				gotest.Equal(t, 1, repo.Count())
			})

			s.Test("get returns correct user", func(t *gotest.T) {
				gotest.Equal(t, "Bob", repo.Get("2"))
			})

			s.Describe("after clearing all", func(s *gotest.S) {
				s.BeforeEach(func(t *gotest.T) {
					repo.Delete("1")
					repo.Delete("2")
				})

				s.Test("repo is empty again", func(t *gotest.T) {
					gotest.Equal(t, 0, repo.Count())
				})
			})
		})
	})
}
