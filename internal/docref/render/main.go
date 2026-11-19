// Command render rewrites the generated sections of the documents from the
// code. With -check it writes nothing, prints each stale section and exits 1
// when there is one.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/internal/docref"
	"github.com/mvrahden/go-test/internal/testkit"
)

func main() {
	check := flag.Bool("check", false, "print the stale sections and write nothing")
	flag.Parse()
	root, err := testkit.RepoRoot()
	if err != nil {
		fail(err)
	}
	if *check {
		stale, err := docref.Stale(root)
		if err != nil {
			fail(err)
		}
		for _, line := range stale {
			fmt.Println(line)
		}
		if len(stale) > 0 {
			os.Exit(1)
		}
		return
	}
	rendered, err := docref.Render(root)
	if err != nil {
		fail(err)
	}
	for _, doc := range docref.Documents {
		path := filepath.Join(root, filepath.FromSlash(doc))
		data, err := os.ReadFile(path) //nolint:gosec // G304: a document of this repository
		if err != nil {
			fail(err)
		}
		if bytes.Equal(data, rendered[doc]) {
			continue
		}
		if err := os.WriteFile(path, rendered[doc], 0o644); err != nil { //nolint:gosec // G306: a committed document
			fail(err)
		}
		fmt.Println("wrote", doc)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "reference:", err)
	os.Exit(1)
}
