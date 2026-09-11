package main

import (
	"fmt"
	"io"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/mvrahden/go-test/internal/gotestast"
	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestspec"
)

// censusCase is one test method at the "TestSuite/TestMethod" level go test
// prints, declared by source or executed by a run.
type censusCase struct {
	Pkg  string
	Path string
}

// forEachEffectiveSuite calls fn for every suite the loaded packages declare,
// after focus and exclusion, in source order. It reads the AST only: a
// harness that forgot a method cannot influence what fn sees.
func forEachEffectiveSuite(loaded []*gotestgen.LoadResult, fn func(pkgPath string, suite *gotestast.TestSuiteSpec)) {
	collector := gotestgen.NewCollector()
	for _, lr := range loaded {
		for _, pkg := range []*packages.Package{lr.Ptest, lr.Pxtest} {
			if pkg == nil {
				continue
			}
			result := collector.CollectSuiteSpecs(pkg)
			if len(result.Errs) > 0 {
				continue
			}
			spec, err := collector.ApplyTestSuiteSpecs(result)
			if err != nil {
				continue
			}
			for _, suite := range spec.EffectiveTestSuites {
				fn(lr.PkgPath, suite)
			}
		}
	}
}

// declaredCases lists every test method as "Test<Suite>/<Method>", the path
// the harness runs it under.
func declaredCases(loaded []*gotestgen.LoadResult) []censusCase {
	var out []censusCase
	forEachEffectiveSuite(loaded, func(pkgPath string, suite *gotestast.TestSuiteSpec) {
		for _, m := range suite.TestCases() {
			out = append(out, censusCase{Pkg: pkgPath, Path: "Test" + suite.Identifier() + "/" + m.Identifier()})
		}
	})
	return out
}

// declaredFuzzCases lists every fuzz target as "Fuzz<Suite>_<Method>", the
// top-level wrapper whose seeds replay as subtests on every run.
func declaredFuzzCases(loaded []*gotestgen.LoadResult) []censusCase {
	var out []censusCase
	forEachEffectiveSuite(loaded, func(pkgPath string, suite *gotestast.TestSuiteSpec) {
		for _, fz := range suite.Fuzzers() {
			out = append(out, censusCase{Pkg: pkgPath, Path: "Fuzz" + suite.Identifier() + "_" + fz.Identifier()})
		}
	})
	return out
}

// declaredBenchCases lists every benchmark method as
// "Benchmark<Suite>/Benchmark<Name>", the path the bench harness runs it under.
func declaredBenchCases(loaded []*gotestgen.LoadResult) []censusCase {
	var out []censusCase
	forEachEffectiveSuite(loaded, func(pkgPath string, suite *gotestast.TestSuiteSpec) {
		for _, m := range suite.Benchmarks() {
			out = append(out, censusCase{Pkg: pkgPath, Path: "Benchmark" + suite.Identifier() + "/" + m.Identifier()})
		}
	})
	return out
}

// executedFuzzCases lists every fuzz wrapper with a terminal verdict. The
// wrapper is a top-level test; its seeds are the subtests beneath it.
func executedFuzzCases(events []gotestspec.TestEvent) []censusCase {
	var out []censusCase
	seen := map[censusCase]bool{}
	for _, ev := range events {
		if !strings.HasPrefix(ev.Test, "Fuzz") || strings.Contains(ev.Test, "/") {
			continue
		}
		switch ev.Action {
		case gotestspec.ActionPass, gotestspec.ActionFail, gotestspec.ActionSkip:
		default:
			continue
		}
		c := censusCase{Pkg: ev.Package, Path: ev.Test}
		if !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	return out
}

// executedBenchCases lists every benchmark that produced a verdict. go test
// emits no pass event for a benchmark: the result line ("ns/op") is its
// verdict, a fail or skip action the other kind. A benchmark that only
// started does not count.
func executedBenchCases(events []gotestspec.TestEvent) []censusCase {
	var out []censusCase
	seen := map[censusCase]bool{}
	for _, ev := range events {
		if ev.Test == "" || !strings.HasPrefix(ev.Test, "Benchmark") || strings.Count(ev.Test, "/") != 1 {
			continue
		}
		switch ev.Action {
		case gotestspec.ActionFail, gotestspec.ActionSkip, gotestspec.ActionBench:
		case gotestspec.ActionOutput:
			if !strings.Contains(ev.Output, " ns/op") {
				continue
			}
		default:
			continue
		}
		c := censusCase{Pkg: ev.Package, Path: ev.Test}
		if !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	return out
}

// executedCases lists every Suite/Method pair with a terminal verdict. A
// suite skipped as a whole appears as a bare suite path; censusMissing treats
// that as covering the suite's methods. Behaviors below the method are not
// censused: they can depend on runtime values.
func executedCases(events []gotestspec.TestEvent) []censusCase {
	var out []censusCase
	seen := map[censusCase]bool{}
	for _, ev := range events {
		// Fuzz wrappers and their seeds are censused as fuzz targets.
		if ev.Test == "" || strings.HasPrefix(ev.Test, "Fuzz") {
			continue
		}
		switch ev.Action {
		case gotestspec.ActionPass, gotestspec.ActionFail, gotestspec.ActionSkip:
		default:
			continue
		}
		depth := strings.Count(ev.Test, "/")
		if depth > 1 || (depth == 0 && ev.Action != gotestspec.ActionSkip) {
			continue
		}
		c := censusCase{Pkg: ev.Package, Path: ev.Test}
		if !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	return out
}

func censusMissing(declared, executed []censusCase) []censusCase {
	ran := map[censusCase]bool{}
	for _, c := range executed {
		ran[c] = true
	}
	var missing []censusCase
	for _, d := range declared {
		if ran[d] {
			continue
		}
		if i := strings.Index(d.Path, "/"); i > 0 && ran[censusCase{Pkg: d.Pkg, Path: d.Path[:i]}] {
			continue
		}
		missing = append(missing, d)
	}
	return missing
}

var (
	testSelectionFlags  = []string{"-run", "-skip", "-list", "-test.run", "-test.skip", "-test.list"}
	benchSelectionFlags = append([]string{"-bench", "-test.bench"}, testSelectionFlags...)
)

// hasSelectionFlag reports whether the go test flags carry one of flags,
// which change which tests or benchmarks run.
func hasSelectionFlag(goTestArgs, flags []string) bool {
	for _, a := range goTestArgs {
		for _, f := range flags {
			if a == f || strings.HasPrefix(a, f+"=") {
				return true
			}
		}
	}
	return false
}

// censusStandsDown reports whether the go test flags change which tests run.
func censusStandsDown(goTestArgs []string) bool {
	return hasSelectionFlag(goTestArgs, testSelectionFlags)
}

// censusGroup is one kind of declared unit with the verdicts a run produced
// for it.
type censusGroup struct {
	Kind               string
	Declared, Executed []censusCase
}

// enforceCensus turns a green run that left declared tests unexecuted into
// exit 2: not a failed test, a run that cannot be believed. Red runs pass
// through, since they are already not false green.
func enforceCensus(w io.Writer, code int, goTestArgs []string, declared, executed []censusCase) int {
	return enforceCensusGroups(w, code, censusStandsDown(goTestArgs), censusGroup{Kind: "test", Declared: declared, Executed: executed})
}

// enforceRunCensus is the census of a test run: every declared test method
// and every declared fuzz target must have produced a verdict.
func enforceRunCensus(w io.Writer, code int, goTestArgs []string, loaded []*gotestgen.LoadResult, events []gotestspec.TestEvent) int {
	return enforceCensusGroups(w, code, censusStandsDown(goTestArgs),
		censusGroup{Kind: "test", Declared: declaredCases(loaded), Executed: executedCases(events)},
		censusGroup{Kind: "fuzz target", Declared: declaredFuzzCases(loaded), Executed: executedFuzzCases(events)})
}

// enforceBenchCensus is the census of a bench run: every declared benchmark
// must have produced a result. -bench selects benchmarks, so it stands the
// census down like -run does.
func enforceBenchCensus(w io.Writer, code int, goTestArgs []string, declared, executed []censusCase) int {
	return enforceCensusGroups(w, code, hasSelectionFlag(goTestArgs, benchSelectionFlags), censusGroup{Kind: "benchmark", Declared: declared, Executed: executed})
}

func enforceCensusGroups(w io.Writer, code int, standsDown bool, groups ...censusGroup) int {
	if code != 0 {
		return code
	}
	if standsDown {
		fmt.Fprintln(w, "note: census skipped under -run/-skip/-list/-bench")
		return code
	}
	for _, g := range groups {
		missing := censusMissing(g.Declared, g.Executed)
		if len(missing) == 0 {
			continue
		}
		fmt.Fprintf(w, "\nFAIL: census: %d declared %s(s) never ran\n", len(missing), g.Kind)
		for _, m := range missing {
			fmt.Fprintf(w, "  %s %s\n", m.Pkg, m.Path)
		}
		code = 2
	}
	return code
}

// noteBenchmarksNotRun says how many declared benchmarks a test run left
// without a result, the way the stdlib note does: silence would imply the
// packages were fully exercised. -bench asked for benchmarks and -run narrowed
// the run, so neither makes the claim and the note stands down.
func noteBenchmarksNotRun(w io.Writer, goTestArgs []string, declared []censusCase, events []gotestspec.TestEvent) {
	if len(declared) == 0 || hasSelectionFlag(goTestArgs, benchSelectionFlags) {
		return
	}
	if n := len(censusMissing(declared, executedBenchCases(events))); n > 0 {
		fmt.Fprintf(w, "note: %d benchmark(s) not run — gotest runs tests; use 'gotest bench'\n", n)
	}
}
