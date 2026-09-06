package main

import (
	"fmt"
	"io"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestspec"
)

// censusCase is one test method at the "TestSuite/TestMethod" level go test
// prints, declared by source or executed by a run.
type censusCase struct {
	Pkg  string
	Path string
}

// declaredCases lists every test method the loaded packages declare, after
// focus and exclusion, in source order. It reads the AST only: a harness that
// forgot a method cannot influence this list.
func declaredCases(loaded []*gotestgen.LoadResult) []censusCase {
	var out []censusCase
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
				for _, m := range suite.TestCases() {
					out = append(out, censusCase{Pkg: lr.PkgPath, Path: "Test" + suite.Identifier() + "/" + m.Identifier()})
				}
			}
		}
	}
	return out
}

// declaredBenchCases lists every benchmark method the loaded packages
// declare, after focus and exclusion, as "Benchmark<Suite>/Benchmark<Name>",
// the path the bench harness runs them under.
func declaredBenchCases(loaded []*gotestgen.LoadResult) []censusCase {
	var out []censusCase
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
				for _, m := range suite.Benchmarks() {
					out = append(out, censusCase{Pkg: lr.PkgPath, Path: "Benchmark" + suite.Identifier() + "/" + m.Identifier()})
				}
			}
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
		if ev.Test == "" {
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

// enforceCensus turns a green run that left declared tests unexecuted into
// exit 2: not a failed test, a run that cannot be believed. Red runs pass
// through, since they are already not false green.
func enforceCensus(w io.Writer, code int, goTestArgs []string, declared, executed []censusCase) int {
	return enforceCensusOf(w, code, censusStandsDown(goTestArgs), "test", declared, executed)
}

// enforceBenchCensus is the census of a bench run: every declared benchmark
// must have produced a result. -bench selects benchmarks, so it stands the
// census down like -run does.
func enforceBenchCensus(w io.Writer, code int, goTestArgs []string, declared, executed []censusCase) int {
	return enforceCensusOf(w, code, hasSelectionFlag(goTestArgs, benchSelectionFlags), "benchmark", declared, executed)
}

func enforceCensusOf(w io.Writer, code int, standsDown bool, kind string, declared, executed []censusCase) int {
	if code != 0 {
		return code
	}
	if standsDown {
		fmt.Fprintln(w, "note: census skipped under -run/-skip/-list/-bench")
		return code
	}
	missing := censusMissing(declared, executed)
	if len(missing) == 0 {
		return code
	}
	fmt.Fprintf(w, "\nFAIL: census: %d declared %s(s) never ran\n", len(missing), kind)
	for _, m := range missing {
		fmt.Fprintf(w, "  %s %s\n", m.Pkg, m.Path)
	}
	return 2
}
