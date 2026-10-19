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

// unescapedPipes counts the pipes of a line no backslash escapes.
func unescapedPipes(line string) int {
	n := 0
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '\\':
			i++
		case '|':
			n++
		}
	}
	return n
}

// A label is one cell: every table row of the live and the static spec has
// the pipes of its header, and none is cut short by a line break.
func (s *CanaryTestSuite) TestSpecMarkdownLabelsStayInTheirCell(t *gotest.T) {
	s.check(t, "specmarkdown")
	for _, c := range []struct {
		static bool
		pipes  int
	}{{false, 4}, {true, 2}} {
		md := s.specMarkdown(t, c.static)
		for _, line := range strings.Split(md, "\n") {
			if strings.HasPrefix(line, "|") && unescapedPipes(line) != c.pipes {
				t.Errorf("specmarkdown: a label breaks its table row (static=%v): %q", c.static, line)
			}
		}
		if !strings.Contains(md, "keeps a\\\\b and a line break in one row |") {
			t.Errorf("specmarkdown: a label with a line break is not one row (static=%v):\n%s", c.static, md)
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
	if !strings.Contains(md, "| **when a path is read** | | |\n| &nbsp;&nbsp;reads a/b paths | PASS | ") {
		t.Errorf("specmarkdown: an It with a slash is split into levels:\n%s", md)
	}
}

// The spec read from source marks what X_ excludes, as the terminal spec does.
func (s *CanaryTestSuite) TestSpecMarkdownMarksExclusions(t *gotest.T) {
	md := s.specMarkdown(t, true)
	for _, want := range []string{"## Parked — EXCLUDED\n", "### Parked — EXCLUDED\n", "never runs — EXCLUDED |"} {
		if !strings.Contains(md, want) {
			t.Errorf("specmarkdown: an excluded node is not marked EXCLUDED: %q missing", want)
		}
	}
}
