package docref

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
)

// Documents are the files that hold generated sections, relative to the repository root.
var Documents = []string{
	"README.md",
	"docs/design/spec.md",
	"skills/writing-gotest-tests/reference/api.md",
	"site/content/reference.html",
}

// allSections are the renderers of every section, read from the code under root.
func allSections(root string) (map[string]Renderer, error) {
	pkgDir := filepath.Join(root, "pkg", "gotest")
	as, err := Assertions(pkgDir)
	if err != nil {
		return nil, err
	}
	config, err := ConfigSections(pkgDir)
	if err != nil {
		return nil, err
	}
	sections := AssertionSections(as)
	maps.Copy(sections, config)
	maps.Copy(sections, FlagSections())
	maps.Copy(sections, EnvSections())
	return sections, nil
}

// Render returns each document as it should read, by path.
func Render(root string) (map[string][]byte, error) {
	sections, err := allSections(root)
	if err != nil {
		return nil, err
	}
	out := map[string][]byte{}
	for _, doc := range Documents {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(doc)))
		if err != nil {
			return nil, err
		}
		rendered, err := Rewrite(data, sections)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", doc, err)
		}
		out[doc] = rendered
	}
	return out, nil
}

// Stale lists "<file>: section <name> is stale; run make reference" for each section whose committed text differs.
func Stale(root string) ([]string, error) {
	rendered, err := Render(root)
	if err != nil {
		return nil, err
	}
	var stale []string
	for _, doc := range Documents {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(doc)))
		if err != nil {
			return nil, err
		}
		committed, err := scan(lines(data))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", doc, err)
		}
		fresh, err := scan(lines(rendered[doc]))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", doc, err)
		}
		was, is := lines(data), lines(rendered[doc])
		for i, p := range committed {
			if body(was, p) != body(is, fresh[i]) {
				stale = append(stale, fmt.Sprintf("%s: section %s is stale; run make reference", doc, p.Section))
			}
		}
	}
	return stale, nil
}

// body is the text between the two markers of p.
func body(ls []string, p pair) string {
	var text string
	for _, l := range ls[p.open+1 : p.end] {
		text += l
	}
	return text
}
