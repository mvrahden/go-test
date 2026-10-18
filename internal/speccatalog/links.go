package speccatalog

import (
	"fmt"
	"path"
	"regexp"
	"strings"
	"unicode"
)

// repoURL is the repository on GitHub.
const repoURL = "https://github.com/mvrahden/go-test/"

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
	bareURL    = regexp.MustCompile(regexp.QuoteMeta(repoURL) + `(?:blob|tree)/[^/\s]+/` + SpecsDir + `\b[^\s)>\]"'` + "`" + `]*`)
	// repoSpec splits a GitHub URL of specs/ on any ref into its path.
	repoSpec = regexp.MustCompile(`^` + regexp.QuoteMeta(repoURL) + `(?:blob|tree)/[^/]+/(` + SpecsDir + `(?:/.*)?)$`)
	heading  = regexp.MustCompile(`^#{1,6}\s+(.*?)\s*#*\s*$`)
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
// gets the first of "-1", "-2", … no heading has taken yet, as on GitHub.
func Anchors(doc []byte) map[string]bool {
	anchors := map[string]bool{}
	repeats := map[string]int{}
	prose(doc, func(_ int, line string) {
		m := heading.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			return
		}
		base := Slug(m[1])
		slug := base
		for anchors[slug] {
			repeats[base]++
			slug = fmt.Sprintf("%s-%d", base, repeats[base])
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
		line = blankCode(line)
		if m := refLink.FindStringSubmatch(line); m != nil {
			add(n, m[1])
			return
		}
		for _, m := range inlineLink.FindAllStringSubmatch(line, -1) {
			add(n, m[1])
		}
		for _, u := range bareURL.FindAllString(inlineLink.ReplaceAllString(line, ""), -1) {
			add(n, strings.TrimRight(u, ".,:;!?"))
		}
	})
	return links
}

// specTarget resolves a link target to a file under specs/ and its anchor.
func specTarget(file, target string) (spec, anchor string, ok bool) {
	p, anchor, _ := strings.Cut(target, "#")
	switch m := repoSpec.FindStringSubmatch(p); {
	case m != nil:
		p = m[1]
	case strings.HasPrefix(p, "/"):
		p = strings.TrimPrefix(p, "/")
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

// blankCode replaces each inline code span of line with spaces. A backtick
// run with no closing run of the same length is literal.
func blankCode(line string) string {
	b := []byte(line)
	run := func(i int) int {
		j := i
		for j < len(b) && b[j] == '`' {
			j++
		}
		return j - i
	}
	for i := 0; i < len(b); {
		n := run(i)
		if n == 0 {
			i++
			continue
		}
		end := -1
		for j := i + n; j < len(b); {
			if m := run(j); m > 0 {
				if m == n {
					end = j + m
					break
				}
				j += m
				continue
			}
			j++
		}
		if end < 0 {
			i += n
			continue
		}
		for k := i; k < end; k++ {
			b[k] = ' '
		}
		i = end
	}
	return string(b)
}

// fence tracks CommonMark code fences line by line: an opener is three or
// more backticks or tildes indented at most three spaces, and only a run of
// the same character, at least as long and followed by nothing but spaces,
// closes it. A backtick opener's info string holds no backtick.
type fence struct {
	char byte
	size int
}

// skip reports whether line opens, sits in or closes a fence.
func (f *fence) skip(line string) bool {
	char, size, rest, ok := fenceRun(line)
	if f.size > 0 {
		if ok && char == f.char && size >= f.size && strings.TrimSpace(rest) == "" {
			f.size = 0
		}
		return true
	}
	if !ok || (char == '`' && strings.Contains(rest, "`")) {
		return false
	}
	f.char, f.size = char, size
	return true
}

// fenceRun splits a line into its fence run of three or more and the rest.
func fenceRun(line string) (char byte, size int, rest string, ok bool) {
	indent := len(line) - len(strings.TrimLeft(line, " "))
	if indent > 3 || indent == len(line) {
		return 0, 0, "", false
	}
	char = line[indent]
	if char != '`' && char != '~' {
		return 0, 0, "", false
	}
	end := indent
	for end < len(line) && line[end] == char {
		end++
	}
	if end-indent < 3 {
		return 0, 0, "", false
	}
	return char, end - indent, line[end:], true
}

// prose calls fn with each line of doc outside code fences, numbered from 1.
// An unterminated fence runs to the end.
func prose(doc []byte, fn func(n int, line string)) {
	var f fence
	text := strings.ReplaceAll(string(doc), "\r\n", "\n")
	for i, line := range strings.Split(text, "\n") {
		if !f.skip(line) {
			fn(i+1, line)
		}
	}
}
