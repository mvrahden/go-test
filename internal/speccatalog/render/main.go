// Command render writes the behavior catalog into docs/design.
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
	md, js, err := speccatalog.Render(context.Background(), root)
	if err != nil {
		fail(err)
	}
	for path, data := range map[string][]byte{speccatalog.MarkdownPath: md, speccatalog.JSONPath: js} {
		if err := os.WriteFile(filepath.Join(root, path), data, 0o644); err != nil { //nolint:gosec // G306: a committed document
			fail(err)
		}
		fmt.Println("wrote", path)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "catalog:", err)
	os.Exit(1)
}
