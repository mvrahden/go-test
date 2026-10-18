// Command clauses prints the clauses the scenarios in specs/ state, one per
// line, as "when <WHEN> → <THEN>". With --unmutated it prints the clauses no
// drill mutant names on a Clause: line, by the same reading the conventions
// suite holds the budget to.
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

func fail(err error) {
	fmt.Fprintln(os.Stderr, "clauses:", err)
	os.Exit(1)
}
