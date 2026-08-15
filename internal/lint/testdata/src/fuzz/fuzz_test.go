package fuzz

import (
	"strconv"
	"strings"
	"time"

	r "math/rand"
	"math/rand/v2"
	"os"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// --- fuzz-determinism ---

type FuzzDeterminismTestSuite struct{}

func (s *FuzzDeterminismTestSuite) FuzzDirectTime(f *gotest.F) {
	f.Add("seed")
	gotest.Fuzz(f, func(t *gotest.T, in string) {
		now := time.Now() // want `fuzz target FuzzDeterminismTestSuite.FuzzDirectTime reads nondeterministic state \(time.Now\) — corpus replay and coverage guidance degrade`
		gotest.NotZero(t, now)
		gotest.Equal(t, in, in)
	})
}

// FuzzAliasedRand imports "math/rand" under the alias r — detection must
// key off the import path, not the identifier text, so this still fires.
func (s *FuzzDeterminismTestSuite) FuzzAliasedRand(f *gotest.F) {
	f.Add(1)
	gotest.Fuzz(f, func(t *gotest.T, in int) {
		n := r.Intn(10) // want `fuzz target FuzzDeterminismTestSuite.FuzzAliasedRand reads nondeterministic state \(r.Intn\) — corpus replay and coverage guidance degrade`
		gotest.GreaterOrEqual(t, n, 0)
		gotest.Equal(t, in, in)
	})
}

func (s *FuzzDeterminismTestSuite) FuzzRandV2(f *gotest.F) {
	f.Add(1)
	gotest.Fuzz(f, func(t *gotest.T, in int) {
		n := rand.IntN(10) // want `fuzz target FuzzDeterminismTestSuite.FuzzRandV2 reads nondeterministic state \(rand.IntN\) — corpus replay and coverage guidance degrade`
		gotest.GreaterOrEqual(t, n, 0)
		gotest.Equal(t, in, in)
	})
}

func (s *FuzzDeterminismTestSuite) FuzzGetenv(f *gotest.F) {
	f.Add("x")
	gotest.Fuzz(f, func(t *gotest.T, in string) {
		v := os.Getenv("FUZZ_SEED") // want `fuzz target FuzzDeterminismTestSuite.FuzzGetenv reads nondeterministic state \(os.Getenv\) — corpus replay and coverage guidance degrade`
		gotest.Equal(t, in, in)
		_ = v
	})
}

// readClock is a same-package helper exactly one level away from the fuzz
// callback (FuzzHelperChain calls it directly), covering the one-level
// receiver/call-follow requirement — a nondeterministic read hidden behind
// a single layer of indirection must still be attributed to its target.
func readClock() time.Time {
	return time.Now() // want `fuzz target FuzzDeterminismTestSuite.FuzzHelperChain reads nondeterministic state \(time.Now\) — corpus replay and coverage guidance degrade`
}

func (s *FuzzDeterminismTestSuite) FuzzHelperChain(f *gotest.F) {
	f.Add("seed")
	gotest.Fuzz(f, func(t *gotest.T, in string) {
		now := readClock()
		gotest.NotZero(t, now)
		gotest.Equal(t, in, in)
	})
}

func (s *FuzzDeterminismTestSuite) FuzzSuppressed(f *gotest.F) {
	f.Add("seed")
	gotest.Fuzz(f, func(t *gotest.T, in string) {
		now := time.Now() //nolint:fuzz-determinism
		gotest.NotZero(t, now)
		gotest.Equal(t, in, in)
	})
}

func (s *FuzzDeterminismTestSuite) FuzzClean(f *gotest.F) {
	f.Add("clean")
	gotest.Fuzz(f, func(t *gotest.T, in string) {
		trimmed := strings.TrimSpace(in)
		gotest.Equal(t, trimmed, strings.TrimSpace(trimmed))
	})
}

// --- fuzz-no-oracle ---

type FuzzNoOracleTestSuite struct{}

func (s *FuzzNoOracleTestSuite) FuzzAssertionFree(f *gotest.F) {
	f.Add("seed")
	gotest.Fuzz(f, func(t *gotest.T, in string) { // want `fuzz target FuzzNoOracleTestSuite.FuzzAssertionFree only detects panics — assert a property \(round-trip, idempotence, no-crash-plus-invariant\)`
		_ = strings.TrimSpace(in)
	})
}

// FuzzErrorCheckOnly checks an error and returns early but asserts nothing
// — "no crash" is not a property, so this still fires, by design.
func (s *FuzzNoOracleTestSuite) FuzzErrorCheckOnly(f *gotest.F) {
	f.Add("42")
	gotest.Fuzz(f, func(t *gotest.T, in string) { // want `fuzz target FuzzNoOracleTestSuite.FuzzErrorCheckOnly only detects panics — assert a property \(round-trip, idempotence, no-crash-plus-invariant\)`
		if _, err := strconv.Atoi(in); err != nil {
			return
		}
	})
}

func (s *FuzzNoOracleTestSuite) FuzzNoErrorClean(f *gotest.F) {
	f.Add("42")
	gotest.Fuzz(f, func(t *gotest.T, in string) {
		_, err := strconv.Atoi(in)
		gotest.NoError(t, err)
	})
}

func (s *FuzzNoOracleTestSuite) FuzzSkipfClean(f *gotest.F) {
	f.Add("")
	gotest.Fuzz(f, func(t *gotest.T, in string) {
		if in == "" {
			t.Skipf("empty input")
		}
		_ = strings.TrimSpace(in)
	})
}

func (s *FuzzNoOracleTestSuite) FuzzSuppressed(f *gotest.F) {
	f.Add("seed")
	gotest.Fuzz(f, func(t *gotest.T, in string) { //nolint:fuzz-no-oracle
		_ = strings.TrimSpace(in)
	})
}

// --- fuzz-seed ---

type FuzzSeedTestSuite struct{}

func (s *FuzzSeedTestSuite) FuzzNoSeed(f *gotest.F) { // want `fuzz target FuzzSeedTestSuite.FuzzNoSeed declares no seeds — coverage-guided exploration starts blind \(table-test harvesting may still seed it\)`
	gotest.Fuzz(f, func(t *gotest.T, in string) {
		trimmed := strings.TrimSpace(in)
		gotest.Equal(t, trimmed, strings.TrimSpace(trimmed))
	})
}

func (s *FuzzSeedTestSuite) FuzzWithSeed(f *gotest.F) {
	f.Add("seed")
	gotest.Fuzz(f, func(t *gotest.T, in string) {
		trimmed := strings.TrimSpace(in)
		gotest.Equal(t, trimmed, strings.TrimSpace(trimmed))
	})
}

func (s *FuzzSeedTestSuite) FuzzSuppressed(f *gotest.F) { //nolint:fuzz-seed
	gotest.Fuzz(f, func(t *gotest.T, in string) {
		trimmed := strings.TrimSpace(in)
		gotest.Equal(t, trimmed, strings.TrimSpace(trimmed))
	})
}

// FuzzDocSuppressed is suppressed via a nolint directive that lives in its
// doc comment (last line of the block) rather than on the same line as the
// declaration — the doc-comment suppression path.
//
//nolint:fuzz-seed
func (s *FuzzSeedTestSuite) FuzzDocSuppressed(f *gotest.F) {
	gotest.Fuzz(f, func(t *gotest.T, in string) {
		trimmed := strings.TrimSpace(in)
		gotest.Equal(t, trimmed, strings.TrimSpace(trimmed))
	})
}

// FuzzUnrelatedComment has a doc comment immediately above it that carries
// no nolint directive — an unrelated preceding comment must not suppress
// the diagnostic.
func (s *FuzzSeedTestSuite) FuzzUnrelatedComment(f *gotest.F) { // want `fuzz target FuzzSeedTestSuite.FuzzUnrelatedComment declares no seeds — coverage-guided exploration starts blind \(table-test harvesting may still seed it\)`
	gotest.Fuzz(f, func(t *gotest.T, in string) {
		trimmed := strings.TrimSpace(in)
		gotest.Equal(t, trimmed, strings.TrimSpace(trimmed))
	})
}

// FuzzBlankLineSeparated has a nolint comment that is separated from the
// declaration by a blank line, so it is not an attached doc comment and
// must not suppress the diagnostic.
//
//nolint:fuzz-seed

func (s *FuzzSeedTestSuite) FuzzBlankLineSeparated(f *gotest.F) { // want `fuzz target FuzzSeedTestSuite.FuzzBlankLineSeparated declares no seeds — coverage-guided exploration starts blind \(table-test harvesting may still seed it\)`
	gotest.Fuzz(f, func(t *gotest.T, in string) {
		trimmed := strings.TrimSpace(in)
		gotest.Equal(t, trimmed, strings.TrimSpace(trimmed))
	})
}

// --- fuzz-struct-corpus ---

// Frame is the struct argument type shared by the fanned targets below —
// a struct's corpus entries are one value per leaf in field order, which is
// what the shape-bound rules key on.
type Frame struct {
	Version uint8
	Topic   string
}

// FuzzStructCorpusTestSuite exercises the on-disk corpus scan: FuzzFrame
// has a fixture entry under testdata/fuzz/FuzzFuzzStructCorpusTestSuite_FuzzFrame/
// (the generated wrapper name is Fuzz<Suite>_<Method>, and this suite's own
// name already starts with Fuzz), FuzzFrameClean has no corpus directory at
// all, and FuzzNativeCorpus has a fixture entry too but fuzzes a
// pass-through type — its corpus files are not bound to any field order and
// must not fire.
type FuzzStructCorpusTestSuite struct{}

func (s *FuzzStructCorpusTestSuite) FuzzFrame(f *gotest.F) { // want `fuzz target FuzzStructCorpusTestSuite.FuzzFrame keeps 1 corpus entry under testdata/fuzz/FuzzFuzzStructCorpusTestSuite_FuzzFrame/ bound to the declaration order of fuzz.Frame's fields — a same-kind reorder silently reinterprets them and an added or removed field rejects them; run gotest fuzz promote to turn them into typed f.Add seeds`
	f.Add(Frame{Version: 1, Topic: "orders"})
	gotest.Fuzz(f, func(t *gotest.T, in Frame) {
		gotest.Equal(t, in, in)
	})
}

func (s *FuzzStructCorpusTestSuite) FuzzFrameClean(f *gotest.F) {
	f.Add(Frame{Version: 2, Topic: "clean"})
	gotest.Fuzz(f, func(t *gotest.T, in Frame) {
		gotest.Equal(t, in, in)
	})
}

func (s *FuzzStructCorpusTestSuite) FuzzNativeCorpus(f *gotest.F) {
	f.Add("x")
	gotest.Fuzz(f, func(t *gotest.T, in string) {
		gotest.Equal(t, in, strings.TrimSuffix(in+"!", "!"))
	})
}

// --- fuzz-hook-io ---

// FuzzHookIOTestSuite declares a fuzz target, so its per-execution hooks
// are throughput-critical.
type FuzzHookIOTestSuite struct{}

func (s *FuzzHookIOTestSuite) BeforeEach(t *gotest.T) {
	_, _ = os.ReadFile("fixture.json") // want `FuzzHookIOTestSuite.BeforeEach replays around every fuzz execution of this suite — os.ReadFile here throttles the fuzzer to IO speed; move it to BeforeAll/AfterAll or a shared fixture`
}

func (s *FuzzHookIOTestSuite) AfterEach(t *gotest.T) {
	time.Sleep(time.Millisecond) // want `FuzzHookIOTestSuite.AfterEach replays around every fuzz execution of this suite — time.Sleep here throttles the fuzzer to IO speed; move it to BeforeAll/AfterAll or a shared fixture`
}

func (s *FuzzHookIOTestSuite) FuzzTrim(f *gotest.F) {
	f.Add("seed")
	gotest.Fuzz(f, func(t *gotest.T, in string) {
		trimmed := strings.TrimSpace(in)
		gotest.Equal(t, trimmed, strings.TrimSpace(trimmed))
	})
}

// FuzzHookIOHelperTestSuite hides the IO one call away — the one-hop
// same-package follow (the fuzz-determinism recipe) must still attribute it.
type FuzzHookIOHelperTestSuite struct{}

func (s *FuzzHookIOHelperTestSuite) BeforeEach(t *gotest.T) {
	s.loadFixture()
}

func (s *FuzzHookIOHelperTestSuite) loadFixture() {
	_, _ = os.Open("fixture.bin") // want `FuzzHookIOHelperTestSuite.BeforeEach replays around every fuzz execution of this suite — os.Open here throttles the fuzzer to IO speed; move it to BeforeAll/AfterAll or a shared fixture`
}

func (s *FuzzHookIOHelperTestSuite) FuzzTrim(f *gotest.F) {
	f.Add("seed")
	gotest.Fuzz(f, func(t *gotest.T, in string) {
		trimmed := strings.TrimSpace(in)
		gotest.Equal(t, trimmed, strings.TrimSpace(trimmed))
	})
}

// NoFuzzHookTestSuite has the same IO-heavy BeforeEach but declares no fuzz
// targets — its hooks run once per test, not per execution, so nothing may
// fire.
type NoFuzzHookTestSuite struct{}

func (s *NoFuzzHookTestSuite) BeforeEach(t *gotest.T) {
	_, _ = os.ReadFile("fixture.json")
}

func (s *NoFuzzHookTestSuite) TestSomething(t *gotest.T) {
	gotest.Equal(t, 1, 1)
}

// --- fuzz-raw-seed ---

type FuzzRawSeedTestSuite struct{}

func (s *FuzzRawSeedTestSuite) FuzzFrameRaw(f *gotest.F) {
	f.Add([]byte("junk")) // want `raw \[\]byte seed on fuzz target FuzzRawSeedTestSuite.FuzzFrameRaw — the target takes fuzz.Frame and gotest.Fuzz rejects a seed of another type; write a typed fuzz.Frame literal instead \(gotest fuzz promote emits one\)`
	f.Add(Frame{Version: 3, Topic: "typed"})
	gotest.Fuzz(f, func(t *gotest.T, in Frame) {
		gotest.Equal(t, in, in)
	})
}

// FuzzBytesNative fuzzes []byte natively — a []byte seed is exactly the
// right shape there and must not fire.
func (s *FuzzRawSeedTestSuite) FuzzBytesNative(f *gotest.F) {
	f.Add([]byte("ok"))
	gotest.Fuzz(f, func(t *gotest.T, in []byte) {
		gotest.Len(t, in, len(in))
	})
}

// FuzzMixedPositions pairs a struct with a native []byte: the rule is
// per-position, so only the seed facing the struct fires and the one facing
// the []byte position stays untouched.
func (s *FuzzRawSeedTestSuite) FuzzMixedPositions(f *gotest.F) {
	f.Add([]byte("junk"), []byte("ok")) // want `raw \[\]byte seed on fuzz target FuzzRawSeedTestSuite.FuzzMixedPositions — the target takes fuzz.Frame and gotest.Fuzz rejects a seed of another type; write a typed fuzz.Frame literal instead \(gotest fuzz promote emits one\)`
	gotest.Fuzz2(f, func(t *gotest.T, in Frame, raw []byte) {
		gotest.Equal(t, in, in)
		gotest.Len(t, raw, len(raw))
	})
}
