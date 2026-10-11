package unidiff_test

import (
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"

	"github.com/mvrahden/go-test/internal/unidiff"
	"github.com/mvrahden/go-test/pkg/gotest"
)

type UnidiffTestSuite struct{}

func (s *UnidiffTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

// lines joins n numbered lines, each with its newline, with the lines named
// in replace swapped for their values.
func lines(n int, replace map[int]string) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		if r, ok := replace[i]; ok {
			b.WriteString(r + "\n")
			continue
		}
		b.WriteString(strconv.Itoa(i) + "\n")
	}
	return b.String()
}

func (s *UnidiffTestSuite) TestDiff(t *gotest.T) {
	diff := func(before, after string) string {
		return unidiff.Diff("a/f", "b/f", []byte(before), []byte(after))
	}

	t.When("nothing changed", func(w *gotest.T) {
		w.It("is empty", func(it *gotest.T) {
			gotest.Empty(it, diff("a\nb\n", "a\nb\n"))
		})
	})

	t.When("one line changed", func(w *gotest.T) {
		w.It("shows it with three lines of context", func(it *gotest.T) {
			gotest.Equal(it, "--- a/f\n+++ b/f\n@@ -2,7 +2,7 @@\n 2\n 3\n 4\n-5\n+five\n 6\n 7\n 8\n",
				diff(lines(10, nil), lines(10, map[int]string{5: "five"})))
		})
	})

	t.When("changes are more than six lines apart", func(w *gotest.T) {
		w.It("splits them into hunks", func(it *gotest.T) {
			gotest.Equal(it, "--- a/f\n+++ b/f\n"+
				"@@ -1,6 +1,6 @@\n 1\n 2\n-3\n+three\n 4\n 5\n 6\n"+
				"@@ -8,7 +8,7 @@\n 8\n 9\n 10\n-11\n+eleven\n 12\n 13\n 14\n",
				diff(lines(14, nil), lines(14, map[int]string{3: "three", 11: "eleven"})))
		})
	})

	t.When("changes are six lines apart", func(w *gotest.T) {
		w.It("shares one hunk", func(it *gotest.T) {
			gotest.Equal(it, "--- a/f\n+++ b/f\n@@ -1,13 +1,13 @@\n 1\n 2\n-3\n+three\n 4\n 5\n 6\n 7\n 8\n 9\n-10\n+ten\n 11\n 12\n 13\n",
				diff(lines(13, nil), lines(13, map[int]string{3: "three", 10: "ten"})))
		})
	})

	t.When("the old file lacks a final newline", func(w *gotest.T) {
		w.It("marks the line", func(it *gotest.T) {
			gotest.Equal(it, "--- a/f\n+++ b/f\n@@ -1,2 +1,2 @@\n a\n-b\n\\ No newline at end of file\n+b\n", diff("a\nb", "a\nb\n"))
		})
	})

	t.When("a side is empty", func(w *gotest.T) {
		w.It("names the line before it, zero", func(it *gotest.T) {
			gotest.Equal(it, "--- a/f\n+++ b/f\n@@ -0,0 +1 @@\n+x\n", diff("", "x\n"))
			gotest.Equal(it, "--- a/f\n+++ b/f\n@@ -1,2 +0,0 @@\n-x\n-y\n", diff("x\ny\n", ""))
		})
	})

	t.When("lines end in CRLF", func(w *gotest.T) {
		w.It("keeps the carriage return in the line", func(it *gotest.T) {
			gotest.Equal(it, "--- a/f\n+++ b/f\n@@ -1,2 +1,2 @@\n a\r\n-b\r\n+c\r\n", diff("a\r\nb\r\n", "a\r\nc\r\n"))
		})
	})
}

func (s *UnidiffTestSuite) TestRoundTrip(t *gotest.T) {
	t.When("random edits are diffed", func(w *gotest.T) {
		rng := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // G404: a fixed seed keeps the cases reproducible
		var failures []string
		for i := range 500 {
			before := randomText(rng)
			after := mutate(rng, before)
			got, err := apply(before, unidiff.Diff("a/f", "b/f", []byte(before), []byte(after)))
			if err != nil || got != after {
				failures = append(failures, fmt.Sprintf("#%d: %v\nbefore %q\nafter  %q\ngot    %q", i, err, before, got, after))
			}
		}

		w.It("patching the old text with the diff yields the new", func(it *gotest.T) {
			gotest.Empty(it, failures)
		})
	})
}

func randomText(rng *rand.Rand) string {
	var b strings.Builder
	for range rng.IntN(30) {
		i := rng.IntN(4)
		b.WriteString("abcd"[i:i+1] + "\n")
	}
	s := b.String()
	if s != "" && rng.IntN(4) == 0 {
		s = s[:len(s)-1] // no final newline
	}
	return s
}

func mutate(rng *rand.Rand, s string) string {
	ls := strings.SplitAfter(s, "\n")
	for range rng.IntN(6) {
		i := rng.IntN(len(ls) + 1)
		switch rng.IntN(3) {
		case 0:
			ls = append(ls[:i], append([]string{"new\n"}, ls[i:]...)...)
		case 1:
			if i < len(ls) {
				ls = append(ls[:i], ls[i+1:]...)
			}
		default:
			if i < len(ls) {
				ls[i] = "changed\n"
			}
		}
	}
	out := strings.Join(ls, "")
	if out != "" && rng.IntN(4) == 0 {
		out = strings.TrimSuffix(out, "\n")
	}
	return out
}

// apply patches before with a unified diff, the way patch does: every
// context and deleted line must match, and hunks must come in order.
func apply(before, diff string) (string, error) {
	if diff == "" {
		return before, nil
	}
	old := strings.SplitAfter(before, "\n")
	if old[len(old)-1] == "" {
		old = old[:len(old)-1]
	}
	rows := strings.SplitAfter(diff, "\n")
	if len(rows) < 3 || rows[0] != "--- a/f\n" || rows[1] != "+++ b/f\n" {
		return "", fmt.Errorf("bad header")
	}
	var out []string
	pos := 0 // next old line to copy
	last := &out
	for _, row := range rows[2:] {
		switch {
		case row == "":
		case strings.HasPrefix(row, "@@ "):
			var oldStart, oldLen, newStart, newLen int
			if _, err := fmt.Sscanf(normalizeHeader(row), "@@ -%d,%d +%d,%d @@\n", &oldStart, &oldLen, &newStart, &newLen); err != nil {
				return "", fmt.Errorf("bad hunk header %q: %w", row, err)
			}
			start := oldStart - 1
			if oldLen == 0 {
				start = oldStart
			}
			if start < pos || start > len(old) {
				return "", fmt.Errorf("hunk out of order: %q", row)
			}
			out = append(out, old[pos:start]...)
			pos = start
		case row == "\\ No newline at end of file\n":
			if len(*last) == 0 {
				return "", fmt.Errorf("marker without a line")
			}
			(*last)[len(*last)-1] = strings.TrimSuffix((*last)[len(*last)-1], "\n")
		case row[0] == ' ' || row[0] == '-':
			if pos >= len(old) || strings.TrimSuffix(old[pos], "\n") != strings.TrimSuffix(row[1:], "\n") {
				return "", fmt.Errorf("line %d does not match %q", pos+1, row)
			}
			if row[0] == ' ' {
				out = append(out, old[pos])
				last = &out
			} else {
				dropped := []string{old[pos]}
				last = &dropped
			}
			pos++
		case row[0] == '+':
			out = append(out, row[1:])
			last = &out
		default:
			return "", fmt.Errorf("bad row %q", row)
		}
	}
	return strings.Join(append(out, old[pos:]...), ""), nil
}

// normalizeHeader spells out the lengths diff -u leaves implied.
func normalizeHeader(h string) string {
	fields := strings.Fields(h)
	for i := 1; i <= 2; i++ {
		if !strings.Contains(fields[i], ",") {
			fields[i] += ",1"
		}
	}
	return strings.Join(fields, " ") + "\n"
}
