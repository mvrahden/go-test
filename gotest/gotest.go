package gotest

import "testing"

// hookFn is the signature for lifecycle hooks.
type hookFn func(*T)

// testEntry represents a registered test case.
type testEntry struct {
	name     string
	fn       func(*T)
	focused  bool
	excluded bool
	parallel bool
}

// describeEntry represents a registered child describe block.
type describeEntry struct {
	name     string
	fn       func(*S)
	focused  bool
	excluded bool
}

// S is the suite builder. It collects tests and hooks during the registration
// phase and executes them during the run phase.
type S struct {
	t          *testing.T
	beforeAll  []hookFn
	afterAll   []hookFn
	beforeEach []hookFn
	afterEach  []hookFn
	tests      []testEntry
	describes  []describeEntry
}

// Run executes a test suite. The provided function registers tests and hooks
// on the S builder. After registration completes, all tests are executed with
// proper lifecycle hook management.
func Run(t *testing.T, fn func(*S)) {
	t.Helper()
	s := &S{t: t}
	fn(s)
	s.run(nil, nil)
}

// BeforeAll registers a hook that runs once before all tests in this suite.
func (s *S) BeforeAll(fn hookFn) { s.beforeAll = append(s.beforeAll, fn) }

// AfterAll registers a hook that runs once after all tests in this suite.
func (s *S) AfterAll(fn hookFn) { s.afterAll = append(s.afterAll, fn) }

// BeforeEach registers a hook that runs before each test in this suite.
func (s *S) BeforeEach(fn hookFn) { s.beforeEach = append(s.beforeEach, fn) }

// AfterEach registers a hook that runs after each test in this suite.
func (s *S) AfterEach(fn hookFn) { s.afterEach = append(s.afterEach, fn) }

// Test registers a named test case.
func (s *S) Test(name string, fn func(*T)) {
	s.tests = append(s.tests, testEntry{name: name, fn: fn})
}

// Describe registers a nested test group with its own hooks.
func (s *S) Describe(name string, fn func(*S)) {
	s.describes = append(s.describes, describeEntry{name: name, fn: fn})
}

func (s *S) FTest(name string, fn func(*T)) {
	s.tests = append(s.tests, testEntry{name: name, fn: fn, focused: true})
}

func (s *S) XTest(name string, fn func(*T)) {
	s.tests = append(s.tests, testEntry{name: name, fn: fn, excluded: true})
}

func (s *S) FDescribe(name string, fn func(*S)) {
	s.describes = append(s.describes, describeEntry{name: name, fn: fn, focused: true})
}

func (s *S) XDescribe(name string, fn func(*S)) {
	s.describes = append(s.describes, describeEntry{name: name, fn: fn, excluded: true})
}

func (s *S) TestParallel(name string, fn func(*T)) {
	s.tests = append(s.tests, testEntry{name: name, fn: fn, parallel: true})
}

func (s *S) run(inheritedBeforeEach, inheritedAfterEach []hookFn) {
	tt := NewT(s.t)

	for _, fn := range s.beforeAll {
		fn(tt)
	}
	defer func() {
		for i := len(s.afterAll) - 1; i >= 0; i-- {
			s.afterAll[i](tt)
		}
	}()

	allBeforeEach := make([]hookFn, 0, len(inheritedBeforeEach)+len(s.beforeEach))
	allBeforeEach = append(allBeforeEach, inheritedBeforeEach...)
	allBeforeEach = append(allBeforeEach, s.beforeEach...)

	allAfterEach := make([]hookFn, 0, len(inheritedAfterEach)+len(s.afterEach))
	allAfterEach = append(allAfterEach, inheritedAfterEach...)
	allAfterEach = append(allAfterEach, s.afterEach...)

	effectiveTests, effectiveDescs := resolveFocus(s.tests, s.describes)

	for _, test := range effectiveTests {
		test := test
		s.t.Run(test.name, func(sub *testing.T) {
			if test.excluded {
				sub.Skip("excluded")
				return
			}
			ttt := NewT(sub)
			for _, fn := range allBeforeEach {
				fn(ttt)
			}
			if test.parallel {
				sub.Parallel()
			}
			defer func() {
				for i := len(allAfterEach) - 1; i >= 0; i-- {
					allAfterEach[i](ttt)
				}
			}()
			test.fn(ttt)
		})
	}

	// Execute child describes.
	for _, desc := range effectiveDescs {
		desc := desc
		s.t.Run(desc.name, func(sub *testing.T) {
			if desc.excluded {
				sub.Skip("excluded")
				return
			}
			child := &S{t: sub}
			desc.fn(child)
			child.run(allBeforeEach, allAfterEach)
		})
	}
}
