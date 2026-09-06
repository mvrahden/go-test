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

// censusStandsDown reports whether the go test flags change which tests run.
func censusStandsDown(goTestArgs []string) bool {
	for _, a := range goTestArgs {
		for _, f := range []string{"-run", "-skip", "-list", "-test.run", "-test.skip", "-test.list"} {
			if a == f || strings.HasPrefix(a, f+"=") {
				return true
			}
		}
	}
	return false
}

// enforceCensus turns a green run that left declared tests unexecuted into
// exit 2: not a failed test, a run that cannot be believed. Red runs pass
// through, since they are already not false green.
func enforceCensus(w io.Writer, code int, goTestArgs []string, declared, executed []censusCase) int {
	if code != 0 {
		return code
	}
	if censusStandsDown(goTestArgs) {
		fmt.Fprintln(w, "note: census skipped under -run/-skip/-list")
		return code
	}
	missing := censusMissing(declared, executed)
	if len(missing) == 0 {
		return code
	}
	fmt.Fprintf(w, "\nFAIL: census: %d declared test(s) never ran\n", len(missing))
	for _, m := range missing {
		fmt.Fprintf(w, "  %s %s\n", m.Pkg, m.Path)
	}
	return 2
}
