---
title: "Failing a Pull Request on a Go Benchmark Regression"
date: 2026-09-16
description: "Turn Go benchmarks into a CI gate: save a baseline, compare a branch against it, and fail the build when a regression is statistically real."
tags: ["Performance"]
keywords: ["go benchmark regression ci", "benchmark gate pull request go", "continuous benchmarking go", "benchstat alternative"]
cta_text: "Put a benchmark gate on your next pull request."
---

Most Go repositories have benchmarks. Far fewer have an answer to the question a reviewer actually asks: *is this pull request slower than main?*

The benchmarks usually run somewhere — a nightly job, a make target, a CI step that prints `ns/op` into a log. The numbers scroll past. Nobody diffs them against last week. A 15% regression lands, and six months later someone bisects a performance complaint back to a commit whose CI was green the whole time.

The gap is not measurement. Go's benchmark tooling is excellent. The gap is that a benchmark result is a number, a pull request needs a verdict, and turning one into the other means storing a baseline, comparing against it, and deciding which differences are real.

`gotest bench` does those three things with `--save`, `--against` and `--gate`.

## Why "is it slower?" is hard to answer

Say you run `go test -bench=. ./...` on main, then on your branch, and put the two outputs side by side. Three problems appear immediately.

**The runs are not comparable.** Different machine, different CPU governor, a noisy neighbour on the CI runner — a 10% difference between two runs can be entirely environmental. Compare a laptop to a runner and the numbers are meaningless.

**One sample is not a measurement.** A single `ns/op` figure is one estimate of a noisy quantity. Rerun the same binary on the same machine and it moves. Without repetitions, you cannot tell 3% of noise from 3% of regression.

**Eyeballing does not scale.** [benchstat](https://pkg.go.dev/golang.org/x/perf/cmd/benchstat) exists precisely because this comparison needs statistics, and it does the statistics well. What it does not do is decide: it prints a table for a human to read. A gate needs a process that exits non-zero.

## Benchmarks as suite methods

In gotest, a benchmark is a method on a suite, the same way a test is:

```go {title="cache_suite_test.go"}
func (s *CacheTestSuite) BenchmarkGetHit(b *gotest.B) {
    key := s.Corpus.Keys[0]
    for b.Loop() {
        s.cache.Get(key)
    }
}
```

The suite's lifecycle hooks apply. `BeforeEach` runs once per benchmark method with the timer stopped, and `AfterEach` after it stops again, so suite setup never lands in the measurement — a hook that sleeps for 50ms leaves a 3ns benchmark reading 3ns. Setup that has to happen *per iteration* is different: that belongs inside the loop, fenced with `b.StopTimer()` and `b.StartTimer()` yourself.

A fixture bound to a benchmark suite may define `BeforeAll`/`AfterAll` only. Per-method fixture hooks assume a fresh invocation per test case, which a benchmark loop does not give them, so the generator rejects them by name rather than emitting code that ignores them.

That `key := s.Corpus.Keys[0]` line above the loop is deliberate. Reading fixture-backed state *inside* `b.Loop()` measures the fixture — if the fixture were a database handle, the loop would time the query. The `bench-fixture-io` lint rule flags exactly that pattern.

Two execution details matter for trustworthy numbers. Benchmark suites dispatch **serially**, one at a time, whatever `--parallel` says — concurrent benchmarks measure each other. And each suite runs as **its own OS process**, so one suite's heap, garbage collector and resident memory do not follow the next one into its measurement, and two suites never compete for the same cores.

## Save a baseline

`--save` writes the run's results as JSON:

{{< terminal title="gotest bench --save=bench.json -count=5 ./..." >}}
BenchmarkCache <span class="t-time">(&lt;1ms)</span>
  <span class="t-pass">✓</span> GetHit   225.7 ns/op · 0 B/op · 0 allocs/op
  <span class="t-pass">✓</span> GetMiss  118.3 ns/op · 0 B/op · 0 allocs/op

1 suite, 2 benchmarks
{{< /terminal >}}

The time in parentheses is the wall clock of the test that drove the benchmark, not the measurement — the per-operation numbers beside each name are. `-count=5` matters too, for a reason the section on regressions below comes back to. The file it writes is small and readable:

```json {title="bench.json"}
{
  "schemaVersion": 1,
  "createdAt": "2026-09-16T09:12:47Z",
  "goVersion": "go1.27.0",
  "goos": "linux",
  "goarch": "amd64",
  "results": [
    {
      "package": "cache",
      "suite": "CacheTestSuite",
      "name": "BenchmarkGetHit",
      "samples": [
        { "iterations": 5116533, "nsPerOp": 225.7, "bytesPerOp": 0, "allocsPerOp": 0 }
      ]
    }
  ]
}
```

One entry per benchmark and one sample per `-count` repetition — the array above is trimmed to a single sample — plus the toolchain and platform that produced them. Those last three fields are what stops you reading an absurd delta table as a regression. Compare an M3 laptop's baseline against a Linux runner and the next section's command says so before the table:

```text
WARN: baseline was recorded in a different environment (goos darwin, now linux;
goarch arm64, now amd64); the deltas compare runs that are not alike
```

It warns and carries on rather than refusing, because comparing across a deliberate toolchain upgrade is a real thing to want, and only you know whether two runners are alike enough to mean something.

## Compare against it

`--against` reads a baseline and prints a delta table beneath the results:

{{< terminal title="gotest bench --against=bench.json -count=5 ./..." >}}
BenchmarkCache <span class="t-time">(&lt;1ms)</span>
  <span class="t-pass">✓</span> GetHit   254.8 ns/op · 0 B/op · 0 allocs/op

BENCHMARK                             OLD ns/op  NEW ns/op  Δ
cache CacheTestSuite/BenchmarkGetHit      225.7      254.8  <span class="t-fail">+12.9% ⚠</span>
{{< /terminal >}}

Benchmarks are matched by package, suite and name. One that exists on only one side is skipped rather than reported as an infinite change — a renamed benchmark is not a regression.

By default the table shows only significant rows. Pass `-v` and every comparison prints, significant or not, which is what you want when you are exploring rather than gating.

## What counts as a regression

This is the part that decides whether a gate is useful or merely annoying.

gotest compares the mean `ns/op` of the baseline samples against the mean of the new samples with **Welch's t-test**, two-tailed, at **p < 0.05**. It needs at least **four samples on each side** to run the test. `-count=5` clears that with one to spare; `-count=1`, the default, does not come close. Below four, it falls back to a blunt rule: a change counts only if it is at least **20%**.

The consequence is worth internalising. During a run for this post, a benchmark moved **+173%** and the gate stayed quiet, because a single outlier iteration inflated the mean while the distributions still overlapped. The percentage is not the decision. The test is.

`--gate=<pct>` turns that into a verdict: the worst *significant* positive change is compared to your threshold, and the run exits 1 if it exceeds it.

{{< terminal title="gotest bench --against=bench.json --gate=10 -count=5 ./..." >}}
BENCHMARK                             OLD ns/op  NEW ns/op  Δ
cache CacheTestSuite/BenchmarkGetHit      225.7      254.8  <span class="t-fail">+12.9% ⚠</span>

<span class="t-fail">bench gate: cache CacheTestSuite/BenchmarkGetHit +12.9% exceeds 10% gate</span>
{{< /terminal >}}

The run exits 1, which is all a CI step needs.

One honest limitation: the comparison and the gate look at `ns/op` only. `B/op` and `allocs/op` are recorded in every baseline and printed in every result, but they do not gate. If allocation count is the number you care about most, keep reading them in review — the gate will not watch them for you.

## In CI

The GitHub Action wires this up with three inputs:

```yaml {title=".github/workflows/bench.yml"}
- uses: mvrahden/go-test@v1
  with:
    bench: true
    bench-baseline: .bench/main.json
    bench-gate: "10"
```

The step runs `gotest bench --spec --json`. Under `--json` the human rendering moves out of the job log entirely: stdout carries the versioned report, which the step captures to a file, and the readable results, delta table and gate verdict go to the job summary. The step sets two outputs: `bench-report`, the path to the JSON document, and `bench-breached-keys`, the comma-joined list of benchmarks that crossed the gate — enough to post a comment naming them, or to fan out to an issue.

That job summary is where a reviewer reads the outcome:

```text
### 1 benchmark ran (6.9s)

| Benchmark | old ns/op | new ns/op | Δ |
|---|---|---|---|
| cache CacheTestSuite/BenchmarkGetHit | 225.7 | 404.7 | +79.3% ⚠ |

**Bench gate breached:** cache CacheTestSuite/BenchmarkGetHit +79.3% exceeds the 10% gate
```

Set the defaults once in `.gotest.yml` and the flags disappear from both the workflow and your shell:

```yaml {title=".gotest.yml"}
bench:
  baseline: .bench/main.json
  gate: 10
```

## Where the baseline comes from

Two approaches, and the choice matters more than the threshold.

**Commit the baseline.** A file in the repository, regenerated deliberately when a change is meant to move the numbers. Reviewable: the diff shows the number changing, in the same pull request that justifies it. The cost is that it is only valid for the machine class that produced it, so regenerate it from CI, not a laptop — the environment warning catches the crudest version of that mistake, but not two Linux runners of different sizes.

**Generate the baseline from main on every run.** Check out main, benchmark it, benchmark the branch, compare. Immune to machine drift because both halves run on the same runner, minutes apart. The cost is double the benchmark time on every pull request.

Start with the committed baseline: it is cheaper and the drift is visible. Move to per-run generation when your runners are heterogeneous enough that the drift starts producing false gates.

Whichever you choose, run at least `-count=4`, and prefer five or more. Below four samples the t-test cannot run at all, and the 20% fallback will either miss real regressions or fire on noise.

## In the editor

The VS Code extension reads the same `--json` document. Each benchmark method gets a CodeLens to run it, and a `5×` lens that runs it with `-count=5` — enough samples for the comparison to be statistically meaningful. Under the method, an annotation shows the last numbers recorded *on this machine's platform*, with a percentage against the baseline only when the CLI marked that delta significant. Hovering draws a sparkline of the method's history, and a breached gate becomes a squiggle on exactly the methods named in `breachedKeys`.

## What to gate

Not everything. A gate on forty benchmarks is a gate that gets disabled the first busy week.

Pick the handful that represent work your users wait for: the hot path of your parser, the serialization on your request path, the cache lookup in your inner loop. Gate those at a threshold you would actually block a merge over — 10% is a reasonable start, 5% needs quiet runners, 20% only catches disasters. Leave the rest ungated: they still run, still print, still land in the baseline, and are there when you go looking.

A gate's job is not to notice every change. It is to make the specific regressions you care about impossible to merge without someone deciding to.

## Further reading

For the CI setup this builds on — summaries, annotations, coverage thresholds — see [Go Tests in GitHub Actions]({{< ref "/blog/gotest-in-ci" >}}). For the fixture model the benchmark suites above rely on, [Test Fixtures in Go]({{< ref "/blog/test-fixtures-in-go" >}}) covers the fundamentals. And if your performance work is about the whole suite rather than one hot path, [Why Your Go Tests Are Slow]({{< ref "/blog/go-testing-at-scale" >}}) looks at the run itself. The full flag surface lives in the [Benchmarking reference](/reference/#benchmarking).
