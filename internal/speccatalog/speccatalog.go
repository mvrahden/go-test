// Package speccatalog renders the behavior catalog: the conformance suites'
// When/It sentences as gotest spec --static reads them from source.
package speccatalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/mvrahden/go-test/internal/testkit"
)

// Paths of the committed catalog, relative to the repository root.
const (
	MarkdownPath = "docs/design/catalog.md"
	JSONPath     = "docs/design/catalog.json"
)

// Pattern selects every conformance package.
const Pattern = "./tests/conformance/..."

// Render reads the conformance suites from source and returns the markdown
// catalog and its JSON twin. The JSON's version field names the CLI build
// that rendered it, so it is dropped: the catalog describes behavior, not
// the binary that read it.
func Render(ctx context.Context, repoRoot string) (md, js []byte, err error) {
	md, err = render(ctx, repoRoot, "md")
	if err != nil {
		return nil, nil, err
	}
	raw, err := render(ctx, repoRoot, "json")
	if err != nil {
		return nil, nil, err
	}
	js, err = withoutVersion(raw)
	if err != nil {
		return nil, nil, err
	}
	return md, js, nil
}

func render(ctx context.Context, repoRoot, format string) ([]byte, error) {
	run, err := testkit.RunCLI(ctx, repoRoot, "spec", "--static", "--format="+format, Pattern)
	if err != nil {
		return nil, err
	}
	if run.Exit != 0 {
		return nil, fmt.Errorf("spec --static exited %d:\n%s", run.Exit, run.Stderr)
	}
	if run.Stdout == "" {
		return nil, errors.New("spec --static printed nothing")
	}
	return []byte(run.Stdout), nil
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
