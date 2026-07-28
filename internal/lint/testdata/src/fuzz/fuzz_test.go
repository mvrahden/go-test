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
