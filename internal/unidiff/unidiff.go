// Package unidiff renders line-based unified diffs, the format patch and
// git apply read.
package unidiff

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
)

// context is the number of unchanged lines around each change.
const context = 3

// Diff returns the unified diff from before to after under the names
// oldName and newName, or "" when they are equal. Lines split on \n alone,
// so a \r stays part of its line.
func Diff(oldName, newName string, before, after []byte) string {
	if bytes.Equal(before, after) {
		return ""
	}
	a, b := splitLines(string(before)), splitLines(string(after))
	ops := script(a, b)

	var out strings.Builder
	fmt.Fprintf(&out, "--- %s\n+++ %s\n", oldName, newName)
	for _, h := range hunks(ops) {
		writeHunk(&out, h, a, b)
	}
	return out.String()
}

// splitLines splits s after each \n; a last line without one is kept.
func splitLines(s string) []string {
	lines := strings.SplitAfter(s, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// op is one line of the edit script: kept (' '), deleted ('-') or inserted
// ('+'). i and j are its indexes into a and b.
type op struct {
	kind byte
	i, j int
}

// script returns a shortest edit script from a to b (Myers' O(ND) diff),
// after trimming the common prefix and suffix.
func script(a, b []string) []op {
	pre := 0
	for pre < len(a) && pre < len(b) && a[pre] == b[pre] {
		pre++
	}
	suf := 0
	for suf < len(a)-pre && suf < len(b)-pre && a[len(a)-1-suf] == b[len(b)-1-suf] {
		suf++
	}

	var ops []op
	for k := range pre {
		ops = append(ops, op{' ', k, k})
	}
	for _, o := range myers(a[pre:len(a)-suf], b[pre:len(b)-suf]) {
		ops = append(ops, op{o.kind, o.i + pre, o.j + pre})
	}
	for k := suf; k > 0; k-- {
		ops = append(ops, op{' ', len(a) - k, len(b) - k})
	}
	return ops
}

// myers computes the edit script of a to b, deletions before insertions.
func myers(a, b []string) []op {
	n, m := len(a), len(b)
	maxD := n + m
	off := maxD + 1
	v := make([]int, 2*maxD+3)
	// trace[d] holds v[off-d-1 : off+d+2] as step d found it.
	var trace [][]int
	for d := 0; d <= maxD; d++ {
		trace = append(trace, slices.Clone(v[off-d-1:off+d+2]))
		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || (k != d && v[off+k-1] < v[off+k+1]) {
				x = v[off+k+1] // down: an insertion
			} else {
				x = v[off+k-1] + 1 // right: a deletion
			}
			y := x - k
			for x < n && y < m && a[x] == b[y] {
				x++
				y++
			}
			v[off+k] = x
			if x >= n && y >= m {
				return backtrack(trace, n, m, d)
			}
		}
	}
	return nil // unreachable: d = n+m always reaches the end
}

// backtrack walks the saved frontiers from (n, m) back to (0, 0).
func backtrack(trace [][]int, n, m, d int) []op {
	var rev []op
	x, y := n, m
	for ; d > 0; d-- {
		prev := trace[d]
		at := func(k int) int { return prev[k+d+1] } // v[k] before step d
		k := x - y
		pk := k - 1
		if k == -d || (k != d && at(k-1) < at(k+1)) {
			pk = k + 1
		}
		px := at(pk)
		py := px - pk
		for x > px && y > py {
			x--
			y--
			rev = append(rev, op{' ', x, y})
		}
		if pk == k+1 {
			y--
			rev = append(rev, op{'+', x, y})
		} else {
			x--
			rev = append(rev, op{'-', x, y})
		}
	}
	for x > 0 && y > 0 {
		x--
		y--
		rev = append(rev, op{' ', x, y})
	}
	slices.Reverse(rev)
	return rev
}

// hunks groups ops into runs of changes with up to context kept lines
// around them; changes closer than 2*context lines share a hunk.
func hunks(ops []op) [][]op {
	var out [][]op
	start, end := -1, -1 // the current hunk, as a range of ops
	for i, o := range ops {
		if o.kind == ' ' {
			continue
		}
		lo, hi := max(i-context, 0), min(i+context+1, len(ops))
		if start >= 0 && lo <= end {
			end = max(end, hi)
			continue
		}
		if start >= 0 {
			out = append(out, ops[start:end])
		}
		start, end = lo, hi
	}
	if start >= 0 {
		out = append(out, ops[start:end])
	}
	return out
}

// writeHunk writes one hunk with its header.
func writeHunk(out *strings.Builder, h []op, a, b []string) {
	var oldStart, newStart, oldLen, newLen int
	oldStart, newStart = -1, -1
	for _, o := range h {
		if o.kind != '+' {
			if oldStart < 0 {
				oldStart = o.i
			}
			oldLen++
		}
		if o.kind != '-' {
			if newStart < 0 {
				newStart = o.j
			}
			newLen++
		}
	}
	// An empty side starts after the line before the hunk.
	if oldStart < 0 {
		oldStart = h[0].i
	}
	if newStart < 0 {
		newStart = h[0].j
	}
	fmt.Fprintf(out, "@@ -%s +%s @@\n", hunkRange(oldStart, oldLen), hunkRange(newStart, newLen))
	for _, o := range h {
		line := ""
		switch o.kind {
		case '-':
			line = a[o.i]
		default:
			line = b[o.j]
		}
		out.WriteByte(o.kind)
		out.WriteString(line)
		if !strings.HasSuffix(line, "\n") {
			out.WriteString("\n\\ No newline at end of file\n")
		}
	}
}

// hunkRange formats a 0-based start and a length as "line,len", the way
// diff -u does: len 1 is implied, and an empty range names the line before.
func hunkRange(start, n int) string {
	switch n {
	case 0:
		return fmt.Sprintf("%d,0", start)
	case 1:
		return fmt.Sprint(start + 1)
	}
	return fmt.Sprintf("%d,%d", start+1, n)
}
