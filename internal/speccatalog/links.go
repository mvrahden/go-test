package speccatalog

import (
	"fmt"
	"path"
	"regexp"
	"strings"
	"unicode"
)

// SpecURL is the GitHub address of specs/ on the default branch.
const SpecURL = "https://github.com/mvrahden/go-test/blob/main/" + SpecsDir + "/"

// SpecLink is a link from a guide into specs/: the spec file it names,
// relative to the repository root, and its anchor without the "#".
type SpecLink struct {
	File   string
	Line   int
	Target string
	Spec   string
	Anchor string
}

var (
	inlineLink = regexp.MustCompile(`\]\(([^)\s]+)[^)]*\)`)
	refLink    = regexp.MustCompile(`^\s*\[[^\]]+\]:\s*(\S+)`)
	bareURL    = regexp.MustCompile(regexp.QuoteMeta(SpecURL) + `[^\s)>\]"'` + "`" + `]*`)
	heading    = regexp.MustCompile(`^#{1,6}\s+(.*?)\s*#*\s*$`)
)

// Slug is the anchor GitHub gives a heading: lowercase, every character but
// letters, digits, "_", "-" and spaces dropped, spaces turned into "-".
func Slug(h string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(h)) {
		switch {
		case r == ' ':
			b.WriteRune('-')
		case r == '-' || r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r):
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Anchors lists the heading anchors of doc outside fences. A repeated slug
// gets "-1", "-2", … as on GitHub.
func Anchors(doc []byte) map[string]bool {
	anchors := map[string]bool{}
	seen := map[string]int{}
	prose(doc, func(_ int, line string) {
		m := heading.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			return
		}
		slug := Slug(m[1])
		if n := seen[slug]; n > 0 {
			seen[slug]++
			slug = fmt.Sprintf("%s-%d", slug, n)
		} else {
			seen[slug] = 1
		}
		anchors[slug] = true
	})
	return anchors
}

// SpecLinks finds the links of doc, outside fences, that lead into specs/:
// inline and reference links relative to file, and GitHub URLs of specs/.
func SpecLinks(file string, doc []byte) []SpecLink {
	var links []SpecLink
	add := func(line int, target string) {
		if spec, anchor, ok := specTarget(file, target); ok {
			links = append(links, SpecLink{File: file, Line: line, Target: target, Spec: spec, Anchor: anchor})
		}
	}
	prose(doc, func(n int, line string) {
		if m := refLink.FindStringSubmatch(line); m != nil {
			add(n, m[1])
			return
		}
		for _, m := range inlineLink.FindAllStringSubmatch(line, -1) {
			add(n, m[1])
		}
		for _, u := range bareURL.FindAllString(inlineLink.ReplaceAllString(line, ""), -1) {
			add(n, u)
		}
	})
	return links
}

// specTarget resolves a link target to a file under specs/ and its anchor.
func specTarget(file, target string) (spec, anchor string, ok bool) {
	p, anchor, _ := strings.Cut(target, "#")
	switch {
	case strings.HasPrefix(p, SpecURL):
		p = strings.TrimPrefix(p, "https://github.com/mvrahden/go-test/blob/main/")
	case p == "" || strings.Contains(p, "://") || strings.HasPrefix(p, "mailto:"):
		return "", "", false
	default:
		p = path.Join(path.Dir(file), p)
	}
	p = path.Clean(p)
	if p != SpecsDir && !strings.HasPrefix(p, SpecsDir+"/") {
		return "", "", false
	}
	return p, anchor, true
}

// prose calls fn with each line of doc outside code fences, numbered from 1.
// A line starting with ``` or ~~~ opens a fence the same marker closes; an
// unterminated fence runs to the end.
func prose(doc []byte, fn func(n int, line string)) {
	var fence string
	text := strings.ReplaceAll(string(doc), "\r\n", "\n")
	for i, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if fence != "" {
			if strings.HasPrefix(trimmed, fence) {
				fence = ""
			}
			continue
		}
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			fence = trimmed[:3]
			continue
		}
		fn(i+1, line)
	}
}
