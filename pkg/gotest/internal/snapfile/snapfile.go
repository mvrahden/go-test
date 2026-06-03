package snapfile

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var headerRe = regexp.MustCompile(`^=== SNAP (.+) ===$`)

// Section represents a single named snapshot section.
type Section struct {
	Key     string
	Content string
}

// Parse parses the snap file format into a slice of Sections.
// Content before the first header is ignored. Returns nil for empty input.
func Parse(data []byte) []Section {
	if len(data) == 0 {
		return nil
	}

	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	text = strings.TrimSuffix(text, "\n")
	lines := strings.Split(text, "\n")

	var sections []Section
	var current *Section

	for _, line := range lines {
		if m := headerRe.FindStringSubmatch(line); m != nil {
			if current != nil {
				sections = append(sections, *current)
			}
			current = &Section{Key: m[1]}
		} else if current != nil {
			current.Content += line + "\n"
		}
	}

	if current != nil {
		sections = append(sections, *current)
	}

	return sections
}

// Serialize serializes sections to the snap file format, sorting by Key.
// Returns nil for empty input.
func Serialize(sections []Section) []byte {
	if len(sections) == 0 {
		return nil
	}

	sorted := make([]Section, len(sections))
	copy(sorted, sections)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Key < sorted[j].Key
	})

	var sb strings.Builder
	for _, s := range sorted {
		fmt.Fprintf(&sb, "=== SNAP %s ===\n", s.Key)
		sb.WriteString(s.Content)
	}

	return []byte(sb.String())
}

// ValidateContent returns an error if content contains a line matching the header pattern.
func ValidateContent(content string) error {
	for line := range strings.SplitSeq(content, "\n") {
		if headerRe.MatchString(line) {
			return fmt.Errorf("content contains a snap header line: %q", line)
		}
	}
	return nil
}
