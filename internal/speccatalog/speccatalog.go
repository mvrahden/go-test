// Package speccatalog reads the behavioral promises in specs/ and binds them
// to the conformance suites' When/It sentences, which it indexes as gotest
// spec --static reads them from source.
package speccatalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/mvrahden/go-test/internal/proctree"
	"github.com/mvrahden/go-test/internal/testkit"
)

// JSONPath is the committed catalog index, relative to the repository root.
const JSONPath = "docs/design/catalog.json"

// Pattern selects every conformance package.
const Pattern = "./tests/conformance/..."

// Render reads the conformance suites from source and returns the catalog
// JSON. Its version field names the CLI build that rendered it, so it is
// dropped: the catalog describes behavior, not the binary that read it.
func Render(ctx context.Context, repoRoot string) ([]byte, error) {
	raw, err := render(ctx, repoRoot)
	if err != nil {
		return nil, err
	}
	return withoutVersion(raw)
}

// render reads the catalog as linux sees the source, on every host: a
// build-constrained conformance file (unix-only, say) would otherwise make the
// committed catalog depend on where it was rendered. What only another
// platform compiles is not catalogued.
func render(ctx context.Context, repoRoot string) ([]byte, error) {
	run, err := testkit.RunCLIWithEnv(ctx, repoRoot, []string{"GOOS=linux"}, "spec", "--static", "--format=json", Pattern)
	if err != nil {
		return nil, err
	}
	if run.Exit != 0 {
		return nil, staticExitError(run.Exit, run.Stderr)
	}
	if run.Stdout == "" {
		return nil, errors.New("spec --static printed nothing")
	}
	return []byte(run.Stdout), nil
}

// staticExitError says spec --static exited with exit, a Windows
// termination status in hex with its name, and what it wrote to stderr.
func staticExitError(exit int, stderr string) error {
	return fmt.Errorf("spec --static exited %s:\n%s", proctree.ExitStatus(exit), stderr)
}

func withoutVersion(raw []byte) ([]byte, error) {
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("spec --static json: %w", err)
	}
	delete(doc, "version")
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}
