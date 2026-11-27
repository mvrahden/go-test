package gotestscenarios

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Normalize is the text a clause and a label compare by: whitespace
// collapsed, backslash escapes, backticks, "**" and "__" dropped, a trailing
// run of ".", ",", ";" or ":" dropped, and the first letter in lower case.
// Every other letter, in any script, stays as written.
func Normalize(s string) string {
	var b strings.Builder
	space := false
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == '\\' && i+1 < len(s) && isASCIIPunct(s[i+1]):
			b.WriteByte(s[i+1])
			i += 2
			space = false
			continue
		case r == '`':
			i++
			continue
		case (r == '*' || r == '_') && i+1 < len(s) && s[i+1] == s[i]:
			i += 2
			continue
		case unicode.IsSpace(r):
			space = b.Len() > 0
			i += size
			continue
		}
		if space {
			b.WriteByte(' ')
			space = false
		}
		b.WriteString(s[i : i+size])
		i += size
	}
	out := strings.TrimRight(b.String(), ".,;: ")
	if r, size := utf8.DecodeRuneInString(out); size > 0 && unicode.IsUpper(r) {
		out = string(unicode.ToLower(r)) + out[size:]
	}
	return out
}

func isASCIIPunct(c byte) bool {
	return c < utf8.RuneSelf && unicode.IsPunct(rune(c)) || strings.IndexByte("$+<=>^`|~", c) >= 0
}
