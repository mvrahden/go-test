package hookcalls

import (
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// A line marked "// at:<Suite>" is where the refusal of <Suite> is located.

// SeededTestSuite has fuzz methods and three hooks that call t.T(); the
// per-input AfterEach gets a real *testing.T under fuzz.
type SeededTestSuite struct{}

func (s *SeededTestSuite) AfterEach(t *gotest.T) { _ = t.T() }

func (s *SeededTestSuite) BeforeAll(t *gotest.T) { _ = t.T() } // at:SeededTestSuite

func (s *SeededTestSuite) BeforeEach(t *gotest.T) {}

func (s *SeededTestSuite) AfterAll(t *gotest.T) { _ = t.T() }

func (s *SeededTestSuite) FuzzSeed(f *gotest.F) { f.Fuzz(func(t *gotest.T, s string) {}) }

// TimedTestSuite has benchmark methods: all four hooks are refused.
type TimedTestSuite struct{}

func (s *TimedTestSuite) AfterEach(t *gotest.T)  { _ = t.T() } // at:TimedTestSuite
func (s *TimedTestSuite) BeforeAll(t *gotest.T)  { _ = t.T() }
func (s *TimedTestSuite) BeforeEach(t *gotest.T) { _ = t.T() }
func (s *TimedTestSuite) AfterAll(t *gotest.T)   { _ = t.T() }
func (s *TimedTestSuite) BenchmarkOne(b *gotest.B) {
	for b.Loop() {
	}
}

// MixedAllTestSuite has both: BeforeAll is nil under either wrapper.
type MixedAllTestSuite struct{}

func (s *MixedAllTestSuite) BeforeAll(t *gotest.T) { _ = t.T() } // at:MixedAllTestSuite
func (s *MixedAllTestSuite) BenchmarkOne(b *gotest.B) {
	for b.Loop() {
	}
}
func (s *MixedAllTestSuite) FuzzOne(f *gotest.F) { f.Fuzz(func(t *gotest.T, s string) {}) }

// MixedEachTestSuite has both: BeforeEach is nil under the benchmark only.
type MixedEachTestSuite struct{}

func (s *MixedEachTestSuite) BeforeEach(t *gotest.T) { _ = t.T() } // at:MixedEachTestSuite
func (s *MixedEachTestSuite) AfterAll(t *gotest.T)   { _ = t.T() }
func (s *MixedEachTestSuite) BenchmarkOne(b *gotest.B) {
	for b.Loop() {
	}
}
func (s *MixedEachTestSuite) FuzzOne(f *gotest.F) { f.Fuzz(func(t *gotest.T, s string) {}) }

type ParenTestSuite struct{}

func (s *ParenTestSuite) BeforeAll(t *gotest.T) { _ = (t).T() } // at:ParenTestSuite
func (s *ParenTestSuite) FuzzOne(f *gotest.F)   { f.Fuzz(func(t *gotest.T, s string) {}) }

type MethodValueTestSuite struct{}

func (s *MethodValueTestSuite) BeforeAll(t *gotest.T) {
	f := t.T // at:MethodValueTestSuite
	_ = f()
}
func (s *MethodValueTestSuite) FuzzOne(f *gotest.F) { f.Fuzz(func(t *gotest.T, s string) {}) }

func use(func() *testing.T) {}

type MethodArgTestSuite struct{}

func (s *MethodArgTestSuite) AfterAll(t *gotest.T) { use(t.T) } // at:MethodArgTestSuite
func (s *MethodArgTestSuite) FuzzOne(f *gotest.F)  { f.Fuzz(func(t *gotest.T, s string) {}) }

type AliasDefineTestSuite struct{}

func (s *AliasDefineTestSuite) BeforeAll(t *gotest.T) {
	tt := t
	_ = tt.T() // at:AliasDefineTestSuite
}
func (s *AliasDefineTestSuite) FuzzOne(f *gotest.F) { f.Fuzz(func(t *gotest.T, s string) {}) }

type AliasVarTestSuite struct{}

func (s *AliasVarTestSuite) BeforeAll(t *gotest.T) {
	var tt = t
	var ttt *gotest.T
	ttt = tt
	_ = ttt.T() // at:AliasVarTestSuite
}
func (s *AliasVarTestSuite) FuzzOne(f *gotest.F) { f.Fuzz(func(t *gotest.T, s string) {}) }

type ClosureTestSuite struct{}

func (s *ClosureTestSuite) BeforeAll(t *gotest.T) {
	func() { _ = t.T() }() // at:ClosureTestSuite
}
func (s *ClosureTestSuite) FuzzOne(f *gotest.F) { f.Fuzz(func(t *gotest.T, s string) {}) }

// UnprovenBindTestSuite checks the result, but not in an if-init.
type UnprovenBindTestSuite struct{}

func (s *UnprovenBindTestSuite) BeforeAll(t *gotest.T) {
	tt := t.T() // at:UnprovenBindTestSuite
	if tt != nil {
		tt.Log("x")
	}
}
func (s *UnprovenBindTestSuite) FuzzOne(f *gotest.F) { f.Fuzz(func(t *gotest.T, s string) {}) }

// WrongBranchTestSuite uses the bound result where it may be nil.
type WrongBranchTestSuite struct{}

func (s *WrongBranchTestSuite) BeforeAll(t *gotest.T) {
	if tt := t.T(); tt != nil { // at:WrongBranchTestSuite
	} else {
		tt.Log("x")
	}
}
func (s *WrongBranchTestSuite) FuzzOne(f *gotest.F) { f.Fuzz(func(t *gotest.T, s string) {}) }

// NilBranchTestSuite calls t.T() in the branch that found it nil.
type NilBranchTestSuite struct{}

func (s *NilBranchTestSuite) BeforeAll(t *gotest.T) {
	if t.T() == nil {
		t.T().Log("x") // at:NilBranchTestSuite
	}
}
func (s *NilBranchTestSuite) FuzzOne(f *gotest.F) { f.Fuzz(func(t *gotest.T, s string) {}) }

// FuzzEachTestSuite's per-input hooks get a real *testing.T under fuzz.
type FuzzEachTestSuite struct{}

func (s *FuzzEachTestSuite) BeforeEach(t *gotest.T) { t.T().Log("x") }
func (s *FuzzEachTestSuite) AfterEach(t *gotest.T)  { t.T().Log("x") }
func (s *FuzzEachTestSuite) FuzzOne(f *gotest.F)    { f.Fuzz(func(t *gotest.T, s string) {}) }

// ShadowTestSuite calls T() on a func literal's own t.
type ShadowTestSuite struct{}

func (s *ShadowTestSuite) BeforeAll(t *gotest.T) {
	run := func(t *gotest.T) { _ = t.T() }
	_ = run
}
func (s *ShadowTestSuite) FuzzOne(f *gotest.F) { f.Fuzz(func(t *gotest.T, s string) {}) }

// OtherTTestSuite calls T() on a *gotest.T that is not the hook's.
type OtherTTestSuite struct{ h *gotest.T }

func (s *OtherTTestSuite) BeforeAll(t *gotest.T) { _ = s.h.T() }
func (s *OtherTTestSuite) FuzzOne(f *gotest.F)   { f.Fuzz(func(t *gotest.T, s string) {}) }

type clock struct{}

func (clock) T() int { return 0 }

// UnrelatedTTestSuite calls a T() method of another type.
type UnrelatedTTestSuite struct{}

func (s *UnrelatedTTestSuite) BeforeAll(t *gotest.T) {
	var c clock
	_ = c.T()
}
func (s *UnrelatedTTestSuite) FuzzOne(f *gotest.F) { f.Fuzz(func(t *gotest.T, s string) {}) }

// GuardedTestSuite uses t.T() only where it was checked non-nil.
type GuardedTestSuite struct{}

func (s *GuardedTestSuite) BeforeAll(t *gotest.T) {
	if tt := t.T(); tt != nil {
		tt.Log("x")
	}
	if tt := t.T(); tt == nil {
	} else {
		tt.Log("x")
	}
	_ = t.T() != nil
	if t.T() != nil && len("x") > 0 {
		t.T().Log("x")
	}
}

func (s *GuardedTestSuite) AfterAll(t *gotest.T) {
	if nil == t.T() {
		return
	}
	t.T().Log("x")
}
func (s *GuardedTestSuite) FuzzOne(f *gotest.F) { f.Fuzz(func(t *gotest.T, s string) {}) }

func logVia(t *gotest.T) { t.T().Log("x") }

// HelperTestSuite reaches t.T() through a helper, which is not followed.
type HelperTestSuite struct{}

func (s *HelperTestSuite) BeforeAll(t *gotest.T) { logVia(t) }
func (s *HelperTestSuite) FuzzOne(f *gotest.F)   { f.Fuzz(func(t *gotest.T, s string) {}) }

// ReturningFuzzTestSuite's returning BeforeEach is refused before its hook.
type ReturningFuzzTestSuite struct{}

type fuzzCtx struct{}

func (s *ReturningFuzzTestSuite) BeforeEach(t *gotest.T) *fuzzCtx { return &fuzzCtx{} }
func (s *ReturningFuzzTestSuite) AfterAll(t *gotest.T)            { _ = t.T() }
func (s *ReturningFuzzTestSuite) FuzzOne(f *gotest.F)             { f.Fuzz(func(t *gotest.T, s string) {}) }

// ReturningBenchTestSuite's returning BeforeEach is refused before its hook.
type ReturningBenchTestSuite struct{}

func (s *ReturningBenchTestSuite) BeforeEach(t *gotest.T) *fuzzCtx { return &fuzzCtx{} }
func (s *ReturningBenchTestSuite) AfterAll(t *gotest.T)            { _ = t.T() }
func (s *ReturningBenchTestSuite) BenchmarkOne(b *gotest.B) {
	for b.Loop() {
	}
}
