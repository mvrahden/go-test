# benchmarking — LRU Cache Hot Path

A fixed-capacity in-process LRU cache sitting in front of a slow store — the
kind of thing almost every service ends up writing, and the one place "is
this allocation-free?" is a question people actually ask about a hot path.

## Structure

- **cache.go** — `Cache`, a hand-rolled intrusive doubly linked list LRU (not
  `container/list`, whose `Element` boxes its payload into `any` and would
  cost an allocation on every promotion)
- **fixtures_test.go** — `KeyCorpusFixture`, a `BeforeAll`-only shared fixture
- **suite_test.go** — `CacheTestSuite`: one correctness test plus four
  benchmarks

## The benchmarks

| Benchmark | Measures |
|---|---|
| `BenchmarkGetHit` | Fetching a key already in the cache — the hot path. Zero allocations. |
| `BenchmarkGetMiss` | Fetching a key that was never cached. |
| `BenchmarkPutEviction` | Inserting into a cache already at capacity — every `Put` evicts the current tail before inserting. |
| `BenchmarkFillFromEmpty` | Building a cache from scratch: one op fills a fresh, exactly-sized cache end to end, so eviction is impossible by construction. |

`BenchmarkFillFromEmpty`'s "op" is a full fill of `fillSize` (4096) entries,
not a single `Put` — its ns/op and allocs/op measure that whole fill, so
don't read it side by side with the single-`Put` numbers from the other
three rows. An earlier version tried to get the same "never evicts"
property from a single cache with headroom (`New(1<<20)`) instead of an
exact-fit fresh cache per op; at default `-benchtime` it ran millions of
iterations, filled that headroom about a fifth of the way through, and then
quietly evicted for the remaining 77% of the run — measuring
`BenchmarkPutEviction`'s path under `BenchmarkPutCold`'s name. Filling a
cache whose capacity exactly equals what gets put into it makes eviction
impossible by construction, independent of how many times `b.Loop()` runs.

## The teaching point: `BeforeEach` is outside the timer

Every benchmark here needs a freshly warmed cache — reusing one left over
from a previous iteration would measure the wrong thing. `CacheTestSuite`'s
`BeforeEach` rebuilds `s.cache` from the corpus before *every* benchmark
method runs, and gotest's generated wrapper fences the timer around that
call: it stops the timer before `BeforeEach`, starts it fresh right before
your `Benchmark*` method body runs, and stops it again before `AfterEach`.
The rebuild is structurally excluded from the measurement.

A hand-written Go benchmark doing the same warm-up needs an explicit
`b.StopTimer()` before rebuilding the cache and `b.StartTimer()` after —
easy to forget, and silent when you do (the benchmark still runs, it just
quietly measures your setup code too). Here there is no timer to remember
to stop; see the comment on `BeforeEach` in `suite_test.go`.

## Why the fixture is `BeforeAll`-only

`KeyCorpusFixture` builds a deterministic 4096-key/value corpus once for the
whole package — every benchmark reads the same keys, which is what makes
`--against` comparisons meaningful run over run. The same corpus warms the
shared `s.cache` used by `BenchmarkGetHit`/`BenchmarkGetMiss`/
`BenchmarkPutEviction`, and its full size is what `BenchmarkFillFromEmpty`
fills per op. `KeyCorpusFixture` defines `BeforeAll` and nothing else.

That's not a style choice: gotest rejects a fixture with `BeforeEach`/
`AfterEach` bound to a suite that has `Benchmark*` methods, at generation
time. Per-method fixture hooks would run *inside* the timed method's
lifecycle — there is no way to fence them out the way the suite's own
`BeforeEach` is fenced — so the generator refuses to build the wrapper
rather than produce a benchmark that silently times someone else's setup.
`BeforeAll`/`AfterAll` run once, outside any benchmark's timing window
entirely, which is the only shape that's safe.

## Running

```
$ go run ./cmd/gotest bench ./examples/benchmarking
goos: linux
goarch: amd64
pkg: github.com/mvrahden/go-test/examples/benchmarking
cpu: AMD Ryzen 9 7950X3D 16-Core Processor          
BenchmarkCacheTestSuite
BenchmarkCacheTestSuite/BenchmarkGetHit
BenchmarkCacheTestSuite/BenchmarkGetHit-6         	98496046	        13.22 ns/op	       0 B/op	       0 allocs/op
BenchmarkCacheTestSuite/BenchmarkGetMiss
BenchmarkCacheTestSuite/BenchmarkGetMiss-6        	160750837	         7.446 ns/op	       0 B/op	       0 allocs/op
BenchmarkCacheTestSuite/BenchmarkPutEviction
BenchmarkCacheTestSuite/BenchmarkPutEviction-6    	 7545878	       150.4 ns/op	      55 B/op	       2 allocs/op
BenchmarkCacheTestSuite/BenchmarkFillFromEmpty
BenchmarkCacheTestSuite/BenchmarkFillFromEmpty-6  	    3754	    318682 ns/op	  415094 B/op	    4114 allocs/op
PASS
ok  	github.com/mvrahden/go-test/examples/benchmarking	4.842s
```

`BenchmarkGetHit` really is 0 B/op, 0 allocs/op — a map lookup plus a
handful of pointer swaps to move the entry to the front of the list, never
a heap allocation.

The two write benchmarks allocate for different reasons, not the same one.
`BenchmarkPutEviction` puts a key that has never existed before on every
call (an ever-incrementing counter through `strconv.Itoa`), so it allocates
a new `*entry` on every call, and a new key string on nearly every call too
(`strconv.Itoa` only avoids allocating for the first 100 integers, out of
the millions this benchmark runs through) — that's 55 B/op here. Its
reported allocs/op flips between 1 and 2 from run to run at that same 55
B/op; that's not the code behaving differently, it's how the metric is
computed. `testing.BenchmarkResult.AllocsPerOp()` is `int64(MemAllocs) /
int64(N)` — integer division, so it can only ever report a whole number.
The true per-call average sits close to 2 (essentially every call allocates
twice) but drifts either side of that boundary run to run, from ordinary
timing and allocation-count variance across a run's iterations — and
integer truncation turns "close to 2" into a clean "1" or "2" depending on
which side it lands on, never a fraction. `BenchmarkFillFromEmpty` reuses
the corpus's pre-built key/value strings, so it never allocates a key; its
4114 allocs/op is consistently just under one `*entry` per `fillSize`
(4096) entries inserted, plus a handful of allocations from the destination
cache's map growing to size as it fills. Both land far from
`BenchmarkGetHit`'s zero — that's the contrast this example exists to show.

### Spec view

```
$ go run ./cmd/gotest bench --spec --no-color ./examples/benchmarking
BenchmarkCache
  ✓ GetHit  9.1 ns/op · 0 B/op · 0 allocs/op
  ✓ GetMiss  7.2 ns/op · 0 B/op · 0 allocs/op
  ✓ PutEviction  141.7 ns/op · 55 B/op · 1 allocs/op
  ✓ FillFromEmpty  291740 ns/op · 415091 B/op · 4114 allocs/op

1 suites, 4 benchmarks: 
```

### Baseline, compare, gate

```
$ go run ./cmd/gotest bench ./examples/benchmarking --save=/tmp/cache-baseline.json -count=6
BenchmarkCache
  ✓ GetHit  9.3 ns/op · 0 B/op · 0 allocs/op
  ✓ GetMiss  7.7 ns/op · 0 B/op · 0 allocs/op
  ✓ PutEviction  146.1 ns/op · 55 B/op · 1 allocs/op
  ✓ FillFromEmpty  328712 ns/op · 415093 B/op · 4114 allocs/op

1 suites, 4 benchmarks: 
```

```
$ go run ./cmd/gotest bench ./examples/benchmarking --against=/tmp/cache-baseline.json
BenchmarkCache
  ✓ GetHit  9.5 ns/op · 0 B/op · 0 allocs/op
  ✓ GetMiss  7.4 ns/op · 0 B/op · 0 allocs/op
  ✓ PutEviction  144.5 ns/op · 55 B/op · 1 allocs/op
  ✓ FillFromEmpty  312995 ns/op · 415093 B/op · 4114 allocs/op

BENCHMARK  OLD ns/op  NEW ns/op  Δ

1 suites, 4 benchmarks: 
```

```
$ go run ./cmd/gotest bench ./examples/benchmarking --against=/tmp/cache-baseline.json --gate=10
BenchmarkCache
  ✓ GetHit  9.0 ns/op · 0 B/op · 0 allocs/op
  ✓ GetMiss  7.0 ns/op · 0 B/op · 0 allocs/op
  ✓ PutEviction  142.4 ns/op · 55 B/op · 2 allocs/op
  ✓ FillFromEmpty  300361 ns/op · 415091 B/op · 4114 allocs/op

BENCHMARK  OLD ns/op  NEW ns/op  Δ

1 suites, 4 benchmarks: 
```

The command above exits `0`.

None of the four benchmarks appear in the delta table in this pair of
runs — every old-vs-new difference here is small enough that it didn't
clear the statistical significance test, so it's correctly reported as
noise rather than a real change, and there's nothing for `--gate=10` to
act on. Had a row cleared significance in the slow direction by more than
10%, it would appear with a trailing `⚠` and the command would exit 1;
`gotest bench`'s own `--against` docs (`gotest help bench`) describe the
same delta table appearing with rows in it when that happens. Deltas alone
never change the exit code — only `--gate` does.

## Why these numbers are trustworthy

- **Serial execution.** `gotest bench` runs benchmark suites one at a time,
  regardless of `--parallel`. Two benchmarks racing for the same CPU
  cores would make both of their timings meaningless.
- **Process-per-suite isolation.** Each suite's benchmarks run in their own
  compiled test binary. GC pressure or heap growth from one suite's
  benchmarks can't leak into another's numbers.
- **`BeforeEach` outside the timer.** Every benchmark above measures only
  the operation named — never the corpus lookup, cache rebuild, or fixture
  hydration that gets it there. See "The teaching point" above.
- **Properties true by construction, not by luck.** `BenchmarkFillFromEmpty`
  never evicts because its cache's capacity exactly equals what gets put
  into it, for every op, regardless of iteration count — not because a
  headroom number happens to outrun a given `-benchtime`. See "The
  benchmarks" above.
