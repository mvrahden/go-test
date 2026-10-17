// Command clauses prints the behavior catalog's clauses, one per line, as
// "<when> → <it>", read from the committed docs/design/catalog.json.
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
	js, err := os.ReadFile(filepath.Join(root, speccatalog.JSONPath)) //nolint:gosec // G304: the committed catalog
	if err != nil {
		fail(err)
	}
	clauses, err := speccatalog.Clauses(js)
	if err != nil {
		fail(err)
	}
	for _, c := range clauses {
		fmt.Println(c)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "clauses:", err)
	os.Exit(1)
}
