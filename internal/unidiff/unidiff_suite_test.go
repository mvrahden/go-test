package unidiff_test

import (
	"fmt"
	"math/rand/v2"
	"runtime"
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

func (s *UnidiffTestSuite) TestHunks(t *gotest.T) {
	hunks := func(before, after string) []unidiff.Hunk {
		return unidiff.Hunks([]byte(before), []byte(after))
	}

	t.When("nothing changed", func(w *gotest.T) {
		w.It("has no hunks", func(it *gotest.T) {
			gotest.Empty(it, hunks("a\nb\n", "a\nb\n"))
		})
	})

	t.When("changes are more than six lines apart", func(w *gotest.T) {
		w.It("returns the hunks Diff prints, without their headers, each with where it starts on both sides", func(it *gotest.T) {
			kept := func(n int) unidiff.Line { return unidiff.Line{Op: ' ', Text: strconv.Itoa(n) + "\n"} }
			gotest.Equal(it, []unidiff.Hunk{
				{A: 0, B: 0, Lines: []unidiff.Line{kept(1), kept(2), {Op: '-', Text: "3\n"}, {Op: '+', Text: "three\n"}, kept(4), kept(5), kept(6)}},
				{A: 7, B: 7, Lines: []unidiff.Line{kept(8), kept(9), kept(10), {Op: '-', Text: "11\n"}, {Op: '+', Text: "eleven\n"}, kept(12), kept(13), kept(14)}},
			}, hunks(lines(14, nil), lines(14, map[int]string{3: "three", 11: "eleven"})))
		})
	})

	t.When("lines were inserted before a hunk", func(w *gotest.T) {
		w.It("starts the hunk at its own line on each side", func(it *gotest.T) {
			before := lines(20, nil)
			after := "new 1\nnew 2\n" + lines(20, map[int]string{15: "fifteen"})
			got := hunks(before, after)
			gotest.Len(it, got, 2)
			gotest.Equal(it, 11, got[1].A)
			gotest.Equal(it, 13, got[1].B)
		})
	})

	t.When("one of many repeated lines is replaced", func(w *gotest.T) {
		before := strings.Repeat("0\n", 20)
		after := strings.Repeat("0\n", 10) + "1\n" + strings.Repeat("0\n", 9)
		w.It("shows the replacement in one place, deletion first", func(it *gotest.T) {
			kept := unidiff.Line{Op: ' ', Text: "0\n"}
			gotest.Equal(it, []unidiff.Hunk{{A: 7, B: 7, Lines: []unidiff.Line{kept, kept, kept, {Op: '-', Text: "0\n"}, {Op: '+', Text: "1\n"}, kept, kept, kept}}},
				hunks(before, after))
		})
		w.It("leaves Diff's patch as it was", func(it *gotest.T) {
			gotest.Equal(it, "--- a/f\n+++ b/f\n@@ -8,6 +8,7 @@\n 0\n 0\n 0\n+1\n 0\n 0\n 0\n@@ -17,4 +18,3 @@\n 0\n 0\n 0\n-0\n",
				unidiff.Diff("a/f", "b/f", []byte(before), []byte(after)))
		})
	})

	t.When("a deleted block could be placed at several lines", func(w *gotest.T) {
		before := "[\n  {\n    1\n  }\n  {\n    2\n  }\n  {\n    3\n  }\n]\n"
		after := "[\n  {\n    1\n  }\n  {\n    3\n  }\n]\n"
		w.It("places it where its first and last lines are least indented", func(it *gotest.T) {
			got := hunks(before, after)
			gotest.Len(it, got, 1)
			var marked []string
			for _, l := range got[0].Lines {
				if l.Op != ' ' {
					marked = append(marked, string(l.Op)+l.Text)
				}
			}
			gotest.Equal(it, []string{"-  {\n", "-    2\n", "-  }\n"}, marked)
		})
	})

	t.When("the last line has no newline", func(w *gotest.T) {
		w.It("keeps it without one", func(it *gotest.T) {
			gotest.Equal(it, []unidiff.Hunk{{Lines: []unidiff.Line{{Op: ' ', Text: "a\n"}, {Op: '-', Text: "b"}, {Op: '+', Text: "c"}}}}, hunks("a\nb", "a\nc"))
		})
	})
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

func (s *UnidiffTestSuite) TestMinimal(t *gotest.T) {
	t.When("random edits are diffed", func(w *gotest.T) {
		rng := rand.New(rand.NewPCG(5, 6)) //nolint:gosec // G404: a fixed seed keeps the cases reproducible
		var failures []string
		for i := range 1000 {
			before := randomText(rng)
			after := mutate(rng, before)
			if i%2 == 1 { // unrelated texts, sharing lines
				before, after = randomLines(rng, rng.IntN(40), 4), randomLines(rng, rng.IntN(40), 4)
			}
			diff := unidiff.Diff("a/f", "b/f", []byte(before), []byte(after))
			want := len(splitLines(before)) - lcsLen(splitLines(before), splitLines(after))
			if got := deletions(diff); got != want {
				failures = append(failures, fmt.Sprintf("#%d: %d deletions, want %d\nbefore %q\nafter  %q", i, got, want, before, after))
			}
		}

		w.It("deletes only the lines outside a longest common subsequence", func(it *gotest.T) {
			gotest.Empty(it, failures)
		})
	})
}

// UnidiffCostCapTestSuite holds the diff correct where the search stops
// looking for a shortest script.
// Sequential: it lowers the package-wide cost floor.
type UnidiffCostCapTestSuite struct {
	restore func()
}

func (s *UnidiffCostCapTestSuite) BeforeAll(t *gotest.T) {
	s.restore = unidiff.SetExpensiveFloor(1)
}

func (s *UnidiffCostCapTestSuite) AfterAll(t *gotest.T) {
	if s.restore != nil {
		s.restore()
	}
}

func (s *UnidiffCostCapTestSuite) TestRoundTrip(t *gotest.T) {
	t.When("random texts are diffed past the cost floor", func(w *gotest.T) {
		rng := rand.New(rand.NewPCG(7, 8)) //nolint:gosec // G404: a fixed seed keeps the cases reproducible
		var failures []string
		longer := 0
		for i := range 500 {
			before, after := randomLines(rng, 200, 4), randomLines(rng, 200, 4)
			diff := unidiff.Diff("a/f", "b/f", []byte(before), []byte(after))
			got, err := apply(before, diff)
			if err != nil || got != after {
				failures = append(failures, fmt.Sprintf("#%d: %v\nbefore %q\nafter  %q\ngot    %q", i, err, before, after, got))
			}
			if deletions(diff) > len(splitLines(before))-lcsLen(splitLines(before), splitLines(after)) {
				longer++
			}
		}

		w.It("gives up on a shortest script for some", func(it *gotest.T) {
			gotest.NotZero(it, longer)
		})
		w.It("patching the old text with the diff still yields the new", func(it *gotest.T) {
			gotest.Empty(it, failures)
		})
	})
}

// UnidiffLargeTestSuite bounds what diffing large files allocates.
// Sequential: it reads the process-wide allocation counter.
type UnidiffLargeTestSuite struct{}

// allocated returns the bytes f allocates.
func allocated(f func()) uint64 {
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	f()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

func (s *UnidiffLargeTestSuite) TestLargeInputs(t *gotest.T) {
	const limit = 64 << 20

	t.When("every line of 20000 changes", func(w *gotest.T) {
		// gofmt rewriting a CRLF file to LF changes every line.
		after := lines(20000, nil)
		before := strings.ReplaceAll(after, "\n", "\r\n")
		var diff string
		n := allocated(func() { diff = unidiff.Diff("a/f", "b/f", []byte(before), []byte(after)) })

		w.It("allocates a bounded amount", func(it *gotest.T) {
			gotest.Less(it, n, uint64(limit))
		})
		w.It("replaces the file in one hunk", func(it *gotest.T) {
			gotest.Equal(it, 20000, deletions(diff))
			gotest.Equal(it, 1, strings.Count(diff, "\n@@ "))
		})
	})

	t.When("20000 lines share content in no useful order", func(w *gotest.T) {
		rng := rand.New(rand.NewPCG(9, 10)) //nolint:gosec // G404: a fixed seed keeps the cases reproducible
		before, after := randomLines(rng, 20000, 10), randomLines(rng, 20000, 10)
		var diff string
		n := allocated(func() { diff = unidiff.Diff("a/f", "b/f", []byte(before), []byte(after)) })
		got, err := apply(before, diff)

		w.It("allocates a bounded amount", func(it *gotest.T) {
			gotest.Less(it, n, uint64(limit))
		})
		w.It("patching the old text with the diff yields the new", func(it *gotest.T) {
			gotest.NoError(it, err)
			gotest.Equal(it, after, got)
		})
	})
}

// randomLines returns n lines drawn from alphabet distinct ones.
func randomLines(rng *rand.Rand, n, alphabet int) string {
	var b strings.Builder
	for range n {
		b.WriteString(strconv.Itoa(rng.IntN(alphabet)) + "\n")
	}
	return b.String()
}

func splitLines(s string) []string {
	ls := strings.SplitAfter(s, "\n")
	if ls[len(ls)-1] == "" {
		ls = ls[:len(ls)-1]
	}
	return ls
}

// lcsLen returns the length of a longest common subsequence, by dynamic
// programming.
func lcsLen(a, b []string) int {
	prev, cur := make([]int, len(b)+1), make([]int, len(b)+1)
	for i := range a {
		for j := range b {
			if a[i] == b[j] {
				cur[j+1] = prev[j] + 1
			} else {
				cur[j+1] = max(prev[j+1], cur[j])
			}
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

// deletions counts a diff's deleted lines.
func deletions(diff string) int {
	n := 0
	rows := strings.Split(diff, "\n")
	for _, row := range rows[min(2, len(rows)):] { // past the header
		if strings.HasPrefix(row, "-") {
			n++
		}
	}
	return n
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
