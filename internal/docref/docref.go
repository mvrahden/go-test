package docref

import (
	"bytes"
	"fmt"
	"html"
	"maps"
	"regexp"
	"slices"
	"strings"
)

// Renderer renders one section in one format. An unknown format is an error
// that reads on from the section's name: `has no format "x"; formats: …`.
type Renderer func(format string) (string, error)

// Marker is one marker pair of a document.
type Marker struct {
	Section, Format string
	Line            int // of the opening marker, 1-based
}

const endMarker = "<!-- gotest:end -->"

var openMarker = regexp.MustCompile(`^<!-- gotest:generated (\S+) (\S+) -->$`)

// pair is a marker pair with the line indexes of its two markers, 0-based.
type pair struct {
	Marker
	open, end int
	indent    string
}

// Sections lists the marker pairs of doc, in order.
func Sections(doc []byte) ([]Marker, error) {
	pairs, err := scan(lines(doc))
	if err != nil {
		return nil, err
	}
	out := make([]Marker, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, p.Marker)
	}
	return out, nil
}

// Rewrite replaces the text between each marker pair of doc with the section's rendering.
func Rewrite(doc []byte, sections map[string]Renderer) ([]byte, error) {
	ls := lines(doc)
	pairs, err := scan(ls)
	if err != nil {
		return nil, err
	}
	eol := "\n"
	if bytes.Contains(doc, []byte("\r\n")) {
		eol = "\r\n"
	}
	var out strings.Builder
	next := 0
	for _, p := range pairs {
		render, ok := sections[p.Section]
		if !ok {
			return nil, fmt.Errorf("line %d: section %q is not known; known: %s", p.Line, p.Section, strings.Join(slices.Sorted(maps.Keys(sections)), ", "))
		}
		text, err := render(p.Format)
		if err != nil {
			return nil, fmt.Errorf("line %d: section %q %w", p.Line, p.Section, err)
		}
		for _, l := range ls[next : p.open+1] {
			out.WriteString(l)
		}
		if text != "" {
			for _, l := range strings.Split(strings.TrimSuffix(text, "\n"), "\n") {
				if l != "" {
					out.WriteString(p.indent + l)
				}
				out.WriteString(eol)
			}
		}
		next = p.end
	}
	for _, l := range ls[next:] {
		out.WriteString(l)
	}
	return []byte(out.String()), nil
}

// formats is the renderer of one section from its renderings by format.
func formats(byFormat map[string]func() string) Renderer {
	return func(format string) (string, error) {
		render, ok := byFormat[format]
		if !ok {
			return "", fmt.Errorf("has no format %q; formats: %s", format, strings.Join(slices.Sorted(maps.Keys(byFormat)), ", "))
		}
		return render(), nil
	}
}

// codeCell escapes a code span for a Markdown table cell.
func codeCell(code string) string {
	return strings.ReplaceAll(code, "|", `\|`)
}

// textEscapes keep prose in a Markdown table cell literal: a table's pipe, a
// tag-like <Name>, and emphasis by * or _.
var textEscapes = strings.NewReplacer("|", `\|`, "<", "&lt;", "*", `\*`, "_", `\_`)

// textCell escapes prose for a Markdown table cell; `code` spans stay as written.
func textCell(text string) string {
	parts := strings.Split(text, "`")
	for i := 0; i < len(parts); i += 2 {
		parts[i] = textEscapes.Replace(parts[i])
	}
	for i := 1; i < len(parts); i += 2 {
		parts[i] = codeCell(parts[i])
	}
	return strings.Join(parts, "`")
}

// htmlText escapes prose for HTML and writes its `code` spans as <code>.
func htmlText(text string) string {
	parts := strings.Split(text, "`")
	var b strings.Builder
	for i, p := range parts {
		switch {
		case i%2 == 0:
			b.WriteString(html.EscapeString(p))
		case i == len(parts)-1: // an unclosed span is text
			b.WriteString("`" + html.EscapeString(p))
		default:
			b.WriteString("<code>" + html.EscapeString(p) + "</code>")
		}
	}
	return b.String()
}

// sentence starts text with a capital letter.
func sentence(text string) string {
	if text == "" {
		return ""
	}
	return strings.ToUpper(text[:1]) + text[1:]
}

// lines splits doc after each line end, keeping the ends.
func lines(doc []byte) []string {
	return strings.SplitAfter(string(doc), "\n")
}

func scan(ls []string) ([]pair, error) {
	var pairs []pair
	var open *pair
	for i, l := range ls {
		text := strings.TrimSpace(l)
		if m := openMarker.FindStringSubmatch(text); m != nil {
			if open != nil {
				return nil, fmt.Errorf("line %d: a marker inside the section opened at line %d; close that one first", i+1, open.Line)
			}
			indent := l[:len(l)-len(strings.TrimLeft(l, " \t"))]
			open = &pair{Marker: Marker{Section: m[1], Format: m[2], Line: i + 1}, open: i, indent: indent}
			continue
		}
		if text == endMarker {
			if open == nil {
				return nil, fmt.Errorf("line %d: %s without an opening marker", i+1, endMarker)
			}
			open.end = i
			pairs = append(pairs, *open)
			open = nil
		}
	}
	if open != nil {
		return nil, fmt.Errorf("line %d: marker is not closed; add %s", open.Line, endMarker)
	}
	return pairs, nil
}
