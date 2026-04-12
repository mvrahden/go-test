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

func (s *S) run(inheritedBeforeEach, inheritedAfterEach []hookFn) {
	// Will be implemented in subsequent tasks.
}
