package fuzzing

import (
	"strings"
	"unicode"
)

// NormalizeTopic canonicalizes a topic name: it lowercases the topic, trims
// surrounding whitespace and '.' separators, and collapses runs of repeated
// '.' into one. The edge trim removes whitespace and '.' together, as a
// single character class, rather than as two separate passes — trimming
// them separately can re-expose the other at a boundary (e.g. trimming '.'
// off ". 0" leaves the space it was hiding), which would make a second
// normalization pass move the string again. Trimming both classes at once
// is what makes NormalizeTopic idempotent by construction:
// NormalizeTopic(NormalizeTopic(s)) == NormalizeTopic(s) for any s.
func NormalizeTopic(topic string) string {
	t := strings.ToLower(topic)
	t = strings.TrimFunc(t, func(r rune) bool {
		return unicode.IsSpace(r) || r == '.'
	})

	var b strings.Builder
	b.Grow(len(t))
	prevDot := false
	for _, r := range t {
		if r == '.' {
			if prevDot {
				continue
			}
			prevDot = true
		} else {
			prevDot = false
		}
		b.WriteRune(r)
	}
	return b.String()
}

// TopicMatches reports whether a and b name the same topic once normalized
// — the comparison a broker's subscription matching actually needs, since
// publishers and subscribers rarely agree on case or separator hygiene.
func TopicMatches(a, b string) bool {
	return NormalizeTopic(a) == NormalizeTopic(b)
}
