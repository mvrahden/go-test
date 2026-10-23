package assert

import (
	"cmp"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/mvrahden/go-test/internal/unidiff"
)

// blockMaxLines is the most lines a rendering may have to be shown whole
// above its diff.
const blockMaxLines = 10

// stringWindow is how many bytes of a long string are shown on each side of
// its first difference.
const stringWindow = 30

// lineMax is the longest line a message shows; a longer one is cut.
const lineMax = 160

// pathMax is the longest path a message names; a longer one keeps its end.
const pathMax = 120

// valuePrefix is the width of "  expected: " and "  actual:   ".
const valuePrefix = "            "

// diffIndent is the width before a diff line's mark.
const diffIndent = "    "

// CheckEqual returns "" if expected and actual are deeply equal. Otherwise
// it returns a message describing the mismatch: short values side by side,
// long single-line strings at their first difference, and anything else as
// a line diff of the values rendered one field, element or entry per line.
// Different dynamic types are named first; renderings that agree are
// rendered again raw before the difference is called hidden.
func CheckEqual(expected, actual any) string {
	if reflect.DeepEqual(expected, actual) {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("Equal failed:\n")
	typed := reflect.TypeOf(expected) != reflect.TypeOf(actual)
	if typed {
		fmt.Fprintf(&sb, "  types differ: %T vs %T\n", expected, actual)
	}
	if sideBySide(&sb, expected, actual, false, typed) {
		return sb.String()
	}
	if m, ok := textMismatch(expected, actual); ok {
		sb.WriteString(m)
		return sb.String()
	}

	re, ra := render(expected, false, typed), render(actual, false, typed)
	if cut := cmp.Or(re.cut, ra.cut); cut != "" {
		fmt.Fprintf(&sb, "  the renderings are cut at %s; the values first differ %s", cut, where(firstDifference(expected, actual)))
		if !re.equal(ra) {
			sb.WriteString("\n")
			writeDiff(&sb, re, ra)
		}
		return sb.String()
	}
	if !re.equal(ra) {
		writeDiff(&sb, re, ra)
		return sb.String()
	}

	// The renderings agree: render again without GoString methods and with
	// sync values opened, which may show what they hid.
	const rawHeading = "  the values render alike; their raw fields differ:\n"
	var raw strings.Builder
	if sideBySide(&raw, expected, actual, true, typed) {
		return sb.String() + rawHeading + raw.String()
	}
	if xe, xa := render(expected, true, typed), render(actual, true, typed); !xe.equal(xa) {
		writeDiff(&raw, xe, xa)
		return sb.String() + rawHeading + raw.String()
	}
	at := ""
	if p := pathText(firstDifference(expected, actual)); p != "" {
		at = " at " + p
	}
	fmt.Fprintf(&sb, "  values differ%s in ways their rendering does not show (NaN, a func, or state the rendering omits)\n", at)
	sb.WriteString("  both:     " + valueBlock(expected, re))
	return sb.String()
}

// sideBySide writes both values when each renders on one short line and
// the two differ.
func sideBySide(sb *strings.Builder, expected, actual any, raw, typed bool) bool {
	ce, okE := compact(expected, compactMax, raw, typed)
	ca, okA := compact(actual, compactMax, raw, typed)
	if !okE || !okA || ce == ca {
		return false
	}
	writeValues(sb, ce, ca)
	return true
}

// where is "at <path>" for a difference below the top, else "at the top".
func where(s *step) string {
	if p := pathText(s); p != "" {
		return "at " + p
	}
	return "at the top"
}

// pathText is s as Go spells it, kept to its last pathMax bytes.
func pathText(s *step) string {
	var tail []string // the last steps, deepest first
	n := 0
	for ; s != nil; s = s.up {
		seg := s.segment()
		if n+len(seg) > pathMax {
			break
		}
		tail = append(tail, seg)
		n += len(seg)
	}
	slices.Reverse(tail)
	switch {
	case s == nil:
		return strings.Join(tail, "")
	case len(tail) == 0:
		more := ""
		if s.up != nil {
			more = "…"
		}
		return more + cutBytes(s.segment(), pathMax) + "…"
	}
	return "…" + strings.Join(tail, "")
}

// writeDiff writes the renderings' diff, with both renderings above it
// when they are small. Two single lines are shown whole when neither is
// cut, else around their first difference with a caret under it.
func writeDiff(sb *strings.Builder, re, ra *rendering) {
	if len(re.lines) == 1 && len(ra.lines) == 1 {
		e, a := re.lines[0], ra.lines[0]
		if len(e) <= lineMax && len(a) <= lineMax {
			writeValues(sb, e, a)
			return
		}
		we, wa, caret, _ := window(e, a, true, false)
		writeValues(sb, we, wa)
		sb.WriteString("\n" + strings.Repeat(" ", len(valuePrefix)+caret) + "^")
		return
	}
	d := valueDiff(re, ra)
	if small(re) && small(ra) {
		writeValues(sb, block(re.lines), block(ra.lines))
		sb.WriteString("\n")
	}
	sb.WriteString(d)
}

// small reports whether a rendering is shown whole above its diff.
func small(r *rendering) bool {
	if len(r.lines) > blockMaxLines {
		return false
	}
	for _, l := range r.lines {
		if len(l) > compactMax {
			return false
		}
	}
	return true
}

// valueDiff marks the lines that differ between two renderings, each hunk
// under the path of its first change.
func valueDiff(re, ra *rendering) string {
	out := []string{"  diff (-expected +actual):"}
	for k, h := range lineHunks(re.lines, ra.lines) {
		if k > 0 {
			out = append(out, diffIndent+"…")
		}
		if p := pathText(hunkPath(h, re, ra)); p != "" {
			out = append(out, diffIndent+"at "+p+":")
		}
		ls := h.Lines
		for n := 0; n < len(ls); {
			end := n + 1
			for ls[n].Op != ' ' && end < len(ls) && ls[end].Op != ' ' {
				end++
			}
			if end == n+2 && ls[n].Op == '-' && ls[n+1].Op == '+' && max(len(chomp(ls[n].Text)), len(chomp(ls[n+1].Text))) > compactMax {
				e, a := ls[n], ls[n+1]
				we, wa, caret, _ := window(chomp(e.Text), chomp(a.Text), true, false)
				out = append(out, markLine(diffIndent, '-', we, e.Text)...)
				out = append(out, markLine(diffIndent, '+', wa, a.Text)...)
				out = append(out, diffIndent+" "+strings.Repeat(" ", caret)+"^")
				n = end
				continue
			}
			for ; n < end; n++ {
				out = append(out, markLine(diffIndent, ls[n].Op, cutLine(chomp(ls[n].Text)), ls[n].Text)...)
			}
		}
	}
	return strings.Join(out, "\n")
}

// hunkPath is the path of a hunk's first changed line. A changed hex row
// that pairs with a row of the other side names its first differing byte.
func hunkPath(h unidiff.Hunk, re, ra *rendering) *step {
	i, j := h.A, h.B
	for n, l := range h.Lines {
		switch l.Op {
		case ' ':
			i, j = i+1, j+1
			continue
		case '+':
			return ra.paths[j]
		}
		s := re.paths[i]
		for _, m := range h.Lines[n+1:] {
			if m.Op == ' ' {
				break
			}
			if m.Op == '+' {
				if o := ra.paths[j]; s != nil && o != nil && s.row && o.row && s.index == o.index {
					return firstByte(s, l.Text, m.Text)
				}
				break
			}
		}
		return s
	}
	return nil
}

// firstByte narrows the path of hex row s to the first byte where its
// texts e and a differ.
func firstByte(s *step, e, a string) *step {
	fe, fa := strings.Fields(e), strings.Fields(a)
	k := 0
	for k < len(fe) && k < len(fa) && fe[k] == fa[k] {
		k++
	}
	return &step{up: s.up, index: s.index + k}
}

// lineHunks diffs two texts given as their lines split at "\n", each
// line keeping its newline. Neither ending in a newline is no difference:
// both are ended in one, so a changed last line is not marked twice.
func lineHunks(expected, actual []string) []unidiff.Hunk {
	if ended(expected) != ended(actual) {
		return unidiff.LineHunks(withNewlines(expected), withNewlines(actual))
	}
	// Every line ends in a newline: diff them without it, sparing a copy.
	hunks := unidiff.LineHunks(trimEnd(expected), trimEnd(actual))
	for _, h := range hunks {
		for k := range h.Lines {
			h.Lines[k].Text += "\n"
		}
	}
	return hunks
}

// ended reports whether lines split at "\n" end in a newline.
func ended(lines []string) bool { return len(lines) > 1 && lines[len(lines)-1] == "" }

// trimEnd is lines without the empty line after a final newline.
func trimEnd(lines []string) []string {
	if ended(lines) {
		return lines[:len(lines)-1]
	}
	return lines
}

// withNewlines is lines each ending in its newline, the last only when
// the text does, cut from one copy of their text.
func withNewlines(lines []string) []string {
	text := strings.Join(lines, "\n")
	out := strings.SplitAfter(text, "\n")
	if out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return out
}

// markLine is a diff line showing shown under op, then a marker when its
// text does not end in a newline.
func markLine(indent string, op byte, shown, text string) []string {
	if strings.HasSuffix(text, "\n") {
		return []string{indent + string(op) + shown}
	}
	return []string{indent + string(op) + shown, indent + `\ No newline at end of file`}
}

// chomp is a line without its newline.
func chomp(s string) string { return strings.TrimSuffix(s, "\n") }

// window shows e and a around their first differing byte at, at most
// stringWindow bytes on each side, a cut side marked "…". indent keeps the
// blanks both lines start with; quote shows the text quoted. caret is the
// column of the difference in what is shown.
func window(e, a string, indent, quote bool) (we, wa string, caret, at int) {
	i := 0
	for i < len(e) && i < len(a) && e[i] == a[i] {
		i++
	}
	for i > 0 && i < len(e) && !utf8.RuneStart(e[i]) {
		i--
	}
	in := 0
	if indent {
		in = min(len(e)-len(strings.TrimLeft(e, " ")), len(a)-len(strings.TrimLeft(a, " ")), i)
	}
	lo := max(i-stringWindow, in)
	for lo > in && !utf8.RuneStart(e[lo]) {
		lo--
	}
	show, mark := func(s string) string { return s }, ""
	if quote {
		show, mark = quoteInner, `"`
	}
	before := show(e[lo:i])
	cut := func(s string) string {
		hi := min(i+stringWindow, len(s))
		for hi < len(s) && !utf8.RuneStart(s[hi]) {
			hi++
		}
		w := s[:in]
		if lo > in {
			w += "…"
		}
		w += mark + before + show(s[i:hi]) + mark
		if hi < len(s) {
			w += "…"
		}
		return w
	}
	caret = in + len(mark) + displayWidth(before)
	if lo > in {
		caret++
	}
	return cut(e), cut(a), caret, i
}

// cutLine keeps a line to lineMax bytes.
func cutLine(s string) string {
	if len(s) <= lineMax {
		return s
	}
	return cutBytes(s, lineMax) + "…"
}

func writeValues(sb *strings.Builder, expected, actual string) {
	fmt.Fprintf(sb, "  expected: %s\n  actual:   %s", expected, actual)
}

// block indents a rendering's lines after its first under the value column.
func block(lines []string) string {
	if n := len(lines); n > 1 && lines[n-1] == "" {
		lines = lines[:n-1] // the newline the text ends in
	}
	cut := make([]string, len(lines))
	for k, l := range lines {
		cut[k] = cutLine(l)
	}
	return strings.Join(cut, "\n"+valuePrefix)
}

// valueBlock shows one value under the value column: on one line when it
// fits, else its first blockMaxLines lines.
func valueBlock(v any, r *rendering) string {
	if c, ok := compact(v, compactMax, false, false); ok {
		return c
	}
	if r == nil {
		r = render(v, false, false)
	}
	if len(r.lines) <= blockMaxLines {
		return block(r.lines)
	}
	return block(r.lines[:blockMaxLines]) + "\n" + valuePrefix + "… " + strconv.Itoa(len(r.lines)-blockMaxLines) + " more lines"
}

// textMismatch shows two single-line strings, or byte slices of UTF-8
// text, around their first differing byte, with a caret under it. Their
// types may differ, not their kind nor their text.
func textMismatch(expected, actual any) (string, bool) {
	e, noun, ok := text(reflect.ValueOf(expected))
	a, nounA, okA := text(reflect.ValueOf(actual))
	if !ok || !okA || noun != nounA || e == a {
		return "", false
	}
	if strings.Contains(strings.TrimSuffix(e, "\n"), "\n") || strings.Contains(strings.TrimSuffix(a, "\n"), "\n") {
		return "", false
	}
	we, wa, caret, i := window(e, a, false, true)
	return fmt.Sprintf("  %s differ at byte %d (expected %d bytes, actual %d)\n  expected: %s\n  actual:   %s\n%s^",
		noun, i, len(e), len(a), we, wa, strings.Repeat(" ", len(valuePrefix)+caret)), true
}

// text is v's text when v is a string or a byte slice of UTF-8 text, and
// the noun for its kind.
func text(v reflect.Value) (s, noun string, ok bool) {
	switch {
	case !v.IsValid():
	case v.Kind() == reflect.String:
		return v.String(), "strings", true
	case v.Kind() == reflect.Slice && v.Type().Elem().Kind() == reflect.Uint8 && utf8.Valid(v.Bytes()):
		return string(v.Bytes()), "byte slices", true
	}
	return "", "", false
}

func quoteInner(s string) string {
	q := strconv.Quote(s)
	return q[1 : len(q)-1]
}

// displayWidth is how many columns s takes in a terminal.
func displayWidth(s string) int {
	n := 0
	for _, r := range s {
		n += runeWidth(r)
	}
	return n
}

// wide are the East Asian wide and fullwidth ranges and the emoji, which
// take two columns.
var wide = [][2]rune{
	{0x1100, 0x115f}, {0x231a, 0x231b}, {0x2329, 0x232a}, {0x23e9, 0x23ec}, {0x23f0, 0x23f0},
	{0x23f3, 0x23f3}, {0x25fd, 0x25fe}, {0x2614, 0x2615}, {0x2648, 0x2653}, {0x267f, 0x267f},
	{0x2693, 0x2693}, {0x26a1, 0x26a1}, {0x26aa, 0x26ab}, {0x26bd, 0x26be}, {0x26c4, 0x26c5},
	{0x26ce, 0x26ce}, {0x26d4, 0x26d4}, {0x26ea, 0x26ea}, {0x26f2, 0x26f3}, {0x26f5, 0x26f5},
	{0x26fa, 0x26fa}, {0x26fd, 0x26fd}, {0x2705, 0x2705}, {0x270a, 0x270b}, {0x2728, 0x2728},
	{0x274c, 0x274c}, {0x274e, 0x274e}, {0x2753, 0x2755}, {0x2757, 0x2757}, {0x2795, 0x2797},
	{0x27b0, 0x27b0}, {0x27bf, 0x27bf}, {0x2b1b, 0x2b1c}, {0x2b50, 0x2b50}, {0x2b55, 0x2b55},
	{0x2e80, 0x303e}, {0x3041, 0x33ff}, {0x3400, 0x4dbf}, {0x4e00, 0x9fff}, {0xa000, 0xa4cf},
	{0xa960, 0xa97f}, {0xac00, 0xd7a3}, {0xf900, 0xfaff}, {0xfe10, 0xfe19}, {0xfe30, 0xfe6f},
	{0xff00, 0xff60}, {0xffe0, 0xffe6}, {0x16fe0, 0x16fe4}, {0x17000, 0x18cff}, {0x1b000, 0x1b2ff},
	{0x1f004, 0x1f004}, {0x1f0cf, 0x1f0cf}, {0x1f18e, 0x1f18e}, {0x1f191, 0x1f19a}, {0x1f200, 0x1f251},
	{0x1f300, 0x1f64f}, {0x1f680, 0x1f6ff}, {0x1f7e0, 0x1f7eb}, {0x1f90c, 0x1f9ff}, {0x1fa70, 0x1faff},
	{0x20000, 0x2fffd}, {0x30000, 0x3fffd},
}

// runeWidth is 2 for a wide rune, 0 for a combining mark, else 1.
func runeWidth(r rune) int {
	if r >= 0x300 && r <= 0x36f || r == 0x200d || r >= 0xfe00 && r <= 0xfe0f {
		return 0
	}
	lo, hi := 0, len(wide)
	for lo < hi {
		m := (lo + hi) / 2
		switch {
		case r < wide[m][0]:
			hi = m
		case r > wide[m][1]:
			lo = m + 1
		default:
			return 2
		}
	}
	return 1
}

// CheckNotEqual returns "" if expected and actual are NOT deeply equal.
// Otherwise it returns a message showing the value both are.
func CheckNotEqual(expected, actual any) string {
	if !reflect.DeepEqual(expected, actual) {
		return ""
	}
	return "NotEqual failed:\n  both are: " + valueBlock(actual, nil)
}
