// Command clauses prints the clauses the scenarios in specs/ state, one per
// line, as "when <WHEN> → <THEN>". With --unmutated it prints the clauses no
// drill mutant names on a Clause: line, by the same reading the conventions
// suite holds the budget to. With --judge-packages <patch> it prints the
// conformance package directories stating the patch's clauses, or nothing
// when the patch changes code a test binary links, and exits 1 on a Clause
// line no scenario states.
package main

import (
	"fmt"
	"os"
	"path/filepath"

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
	if len(os.Args) > 2 && os.Args[1] == "--judge-packages" {
		judgePackages(specs, os.Args[2])
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

func judgePackages(specs []speccatalog.Spec, patchPath string) {
	patch, err := os.ReadFile(patchPath) //nolint:gosec // G304: a drill mutant named on the command line
	if err != nil {
		fail(err)
	}
	pkgs, unknown := speccatalog.PackagesOf(specs, patch)
	for _, c := range unknown {
		fmt.Fprintf(os.Stderr, "clauses: %s: unknown clause: %s\n", patchPath, c)
	}
	if len(unknown) > 0 {
		os.Exit(1)
	}
	if speccatalog.LinksIntoTestBinaries(patch) {
		return
	}
	for _, p := range pkgs {
		fmt.Println(p)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "clauses:", err)
	os.Exit(1)
}
