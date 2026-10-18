package gotestgen

import (
	"bytes"
	"go/build/constraint"
	"strings"
)

// tagsFromBuildFlags reads the -tags values out of go build flags.
func tagsFromBuildFlags(flags []string) []string {
	var tags []string
	seen := map[string]bool{}
	add := func(list string) {
		for _, tag := range strings.FieldsFunc(list, func(r rune) bool { return r == ',' || r == ' ' }) {
			if !seen[tag] {
				seen[tag] = true
				tags = append(tags, tag)
			}
		}
	}
	for i := 0; i < len(flags); i++ {
		name, value, hasValue := strings.Cut(strings.TrimPrefix(flags[i], "-"), "=")
		if name != "tags" && name != "-tags" {
			continue
		}
		if !hasValue && i+1 < len(flags) {
			i++
			value = flags[i]
		}
		add(value)
	}
	return tags
}

// withBuildTags constrains src to the tags it was generated under, joined
// with any //go:build line src already opens with. Without tags src is
// returned as is.
func withBuildTags(src []byte, tags []string) []byte {
	if len(tags) == 0 {
		return src
	}
	var expr constraint.Expr
	for _, tag := range tags {
		expr = and(expr, &constraint.TagExpr{Tag: tag})
	}
	rest := src
	if line, after, ok := bytes.Cut(src, []byte("\n")); ok && constraint.IsGoBuild(string(line)) {
		if existing, err := constraint.Parse(string(line)); err == nil {
			expr = and(existing, expr)
			rest = bytes.TrimLeft(after, "\n")
		}
	}
	out := []byte("//go:build " + expr.String() + "\n\n")
	return append(out, rest...)
}

func and(x, y constraint.Expr) constraint.Expr {
	if x == nil {
		return y
	}
	return &constraint.AndExpr{X: x, Y: y}
}
