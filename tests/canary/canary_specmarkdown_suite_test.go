package canary_test //nolint:fail-guard

import (
	"bytes"
	"errors"
	"os/exec"
	"strings"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// specMarkdown runs the CLI's spec over the specmarkdown fixture in Markdown,
// live or read from source, and returns what it printed.
func (s *CanaryTestSuite) specMarkdown(t *gotest.T, static bool) string {
	args := []string{"spec", "--format=md"}
	if static {
		args = append(args, "--static")
	}
	cmd := exec.Command(s.binary, append(args, "./testdata/specmarkdown/")...) //nolint:gosec // G204: controlled binary with fixed args
	cmd.Env = childEnv(t.TempDir())
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			fatalf(t, "spec %v: %v", args, err)
		}
		t.Errorf("specmarkdown: spec %v exited %d\n%s", args, exitErr.ExitCode(), stderr.String())
	}
	return stdout.String()
}

// strayLine is the first line of md below its first heading that is none of
// a heading, a list item, a table row or a code block line; "" when there is
// none. A label that breaks its line leaves one.
func strayLine(md string) string {
	headed, fenced := false, false
	for _, line := range strings.Split(md, "\n") {
		text := strings.TrimLeft(line, " ")
		switch {
		case strings.HasPrefix(text, "```"):
			fenced = !fenced
		case fenced, text == "", strings.HasPrefix(text, "- "), strings.HasPrefix(text, "|"):
		case strings.HasPrefix(line, "## "):
			headed = true
		case strings.HasPrefix(line, "#"):
		case headed || len(text) != len(line):
			return line
		}
	}
	return ""
}

// A label is one line: no line of the live or the static spec is left over
// from a label that broke its item.
func (s *CanaryTestSuite) TestSpecMarkdownLabelsStayOnTheirLine(t *gotest.T) {
	s.check(t, "specmarkdown")
	for _, static := range []bool{false, true} {
		md := s.specMarkdown(t, static)
		if line := strayLine(md); line != "" {
			t.Errorf("specmarkdown: a label breaks its line (static=%v): %q", static, line)
		}
		if !strings.Contains(md, "keeps a\\\\b and a line break in one row\n") {
			t.Errorf("specmarkdown: a label with a line break is not one line (static=%v):\n%s", static, md)
		}
	}
}

// A label is text: no tag of it reaches the reader as HTML and no backtick
// of it opens a code span.
func (s *CanaryTestSuite) TestSpecMarkdownLabelsAreNotHTML(t *gotest.T) {
	for _, static := range []bool{false, true} {
		md := s.specMarkdown(t, static)
		for _, raw := range []string{"<b>", "</b>", "<i>", "</i>", "and `code`"} {
			if strings.Contains(md, raw) {
				t.Errorf("specmarkdown: a label reaches the reader as markup (static=%v): %q", static, raw)
			}
		}
	}
}

// go test reports one subtest for an It whose label holds a slash, so the
// live spec shows one It under its When.
func (s *CanaryTestSuite) TestSpecMarkdownASlashIsNoLevel(t *gotest.T) {
	md := s.specMarkdown(t, false)
	if !strings.Contains(md, "\n  - when a path is read\n    - PASS reads a/b paths\n") {
		t.Errorf("specmarkdown: an It with a slash is split into levels:\n%s", md)
	}
}

// The spec read from source marks what X_ excludes, as the terminal spec does.
func (s *CanaryTestSuite) TestSpecMarkdownMarksExclusions(t *gotest.T) {
	md := s.specMarkdown(t, true)
	for _, want := range []string{"#### Parked — EXCLUDED\n", "- **Parked** — EXCLUDED\n", "- never runs — EXCLUDED\n"} {
		if !strings.Contains(md, want) {
			t.Errorf("specmarkdown: an excluded node is not marked EXCLUDED: %q missing", want)
		}
	}
}
