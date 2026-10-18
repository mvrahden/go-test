// Command render writes the JSON index of the conformance When and It labels
// into docs/design.
package main

import (
	"context"
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
	js, err := speccatalog.Render(context.Background(), root)
	if err != nil {
		fail(err)
	}
	if err := os.WriteFile(filepath.Join(root, speccatalog.JSONPath), js, 0o644); err != nil { //nolint:gosec // G306: a committed document
		fail(err)
	}
	fmt.Println("wrote", speccatalog.JSONPath)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "catalog:", err)
	os.Exit(1)
}
