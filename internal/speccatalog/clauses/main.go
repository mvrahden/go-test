// Command clauses prints the clauses the scenarios in specs/ state, one per
// line, as "<WHEN> → <THEN>" with the WHEN spelled as gotest spec renders a
// When label. With --unmutated it prints the clauses no
// drill mutant names on a Clause: line, by the same reading the conventions
// suite holds the budget to. With --judge-runs <patch>... it prints, for each
// patch, the judge runs speccatalog.JudgeRuns reads off its header, and exits
// 1 when a patch cannot be judged as its header stands.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mvrahden/go-test/internal/speccatalog"
	"github.com/mvrahden/go-test/internal/testkit"
)

func main() {
	root, err := testkit.RepoRoot()
	if err != nil {
		fail(err)
	}
	specs, err := speccatalog.LoadSpecs(root)
	if err != nil {
		fail(err)
	}
	if len(os.Args) > 2 && os.Args[1] == "--judge-runs" {
		judgeRuns(root, specs, os.Args[2:])
		return
	}
	clauses := speccatalog.Clauses(specs)
	if len(os.Args) > 1 && os.Args[1] == "--unmutated" {
		clauses, _, err = speccatalog.Unmutated(clauses, filepath.Join(root, "tests", "drill", "mutants"))
		if err != nil {
			fail(err)
		}
	}
	for _, c := range clauses {
		fmt.Println(c)
	}
}

// judgeRuns prints one "mutant" line per patch, then one "run" line per
// judge run, tab-separated, with "-" for a run that selects every test:
//
//	mutant	<name>	alone|shared
//	run	<name>	<-run pattern>	<package>...
//
// A patch that cannot be judged gets "bad	<name>	<reason>" lines instead.
func judgeRuns(root string, specs []speccatalog.Spec, patches []string) {
	catalog, err := os.ReadFile(filepath.Join(root, speccatalog.JSONPath))
	if err != nil {
		fail(err)
	}
	bad := false
	for _, path := range patches {
		name := strings.TrimSuffix(filepath.Base(path), ".patch")
		patch, err := os.ReadFile(path) //nolint:gosec // G304: a drill mutant named on the command line
		if err != nil {
			fail(err)
		}
		runs, problems := speccatalog.JudgeRuns(specs, catalog, patch)
		alone := speccatalog.AskedAlone(patch)
		for _, r := range runs {
			a, p, err := r.Alone(root)
			if err != nil {
				fail(err)
			}
			alone = alone || a
			problems = append(problems, p...)
		}
		if len(problems) > 0 {
			bad = true
			for _, p := range problems {
				fmt.Printf("bad\t%s\t%s\n", name, p)
			}
			continue
		}
		mode := "shared"
		if alone {
			mode = "alone"
		}
		fmt.Printf("mutant\t%s\t%s\n", name, mode)
		for _, r := range runs {
			filter := r.RunFilter()
			if filter == "" {
				filter = "-"
			}
			fmt.Printf("run\t%s\t%s\t%s\n", name, filter, strings.Join(r.Packages, " "))
		}
	}
	if bad {
		os.Exit(1)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "clauses:", err)
	os.Exit(1)
}
