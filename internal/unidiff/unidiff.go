// Package unidiff renders line-based unified diffs, the format patch and
// git apply read.
package unidiff

import (
	"bytes"
	"fmt"
	"math"
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

// script returns an edit script from a to b, deletions before insertions
// within each change. It is a shortest one unless the inputs are so
// different that the search gives up on minimality to stay fast.
func script(a, b []string) []op {
	ia, ib := intern(a, b)
	var ops []op
	i, j := 0, 0
	for _, p := range common(ia, ib) {
		for ; i < p.i; i++ {
			ops = append(ops, op{'-', i, j})
		}
		for ; j < p.j; j++ {
			ops = append(ops, op{'+', i, j})
		}
		ops = append(ops, op{' ', i, j})
		i, j = i+1, j+1
	}
	for ; i < len(a); i++ {
		ops = append(ops, op{'-', i, j})
	}
	for ; j < len(b); j++ {
		ops = append(ops, op{'+', i, j})
	}
	return ops
}

// expensiveFloor is the least edit cost at which a split gives up on
// minimality; it grows with the square root of the input, as in GNU diff.
var expensiveFloor = 4096

// intern numbers the distinct lines of a and b.
func intern(a, b []string) (ia, ib []int) {
	ids := map[string]int{}
	num := func(lines []string) []int {
		out := make([]int, len(lines))
		for k, l := range lines {
			id, ok := ids[l]
			if !ok {
				id = len(ids)
				ids[l] = id
			}
			out[k] = id
		}
		return out
	}
	return num(a), num(b)
}

// pair is a line kept from a[i] to b[j].
type pair struct{ i, j int }

// common returns a longest common subsequence of a and b as index pairs,
// in order. A line only one side has can be in none, so the search runs
// over the lines both have: a file rewritten wholesale costs no search.
func common(a, b []int) []pair {
	inA, inB := map[int]bool{}, map[int]bool{}
	for _, x := range a {
		inA[x] = true
	}
	for _, y := range b {
		inB[y] = true
	}
	var sa, sb []int // the shared lines
	var ma, mb []int // their indexes in a and b
	for k, x := range a {
		if inB[x] {
			sa, ma = append(sa, x), append(ma, k)
		}
	}
	for k, y := range b {
		if inA[y] {
			sb, mb = append(sb, y), append(mb, k)
		}
	}

	n, m := len(sa), len(sb)
	s := &search{a: sa, b: sb, fd: make([]int, n+m+3), bd: make([]int, n+m+3), off: m + 1}
	grow := 1
	for d := n + m + 3; d != 0; d >>= 2 {
		grow <<= 1
	}
	s.expensive = max(expensiveFloor, grow)
	s.compare(0, n, 0, m)
	for k := range s.out {
		s.out[k] = pair{ma[s.out[k].i], mb[s.out[k].j]}
	}
	return s.out
}

// search finds a longest common subsequence in linear space, after Myers'
// "An O(ND) Difference Algorithm and Its Variations" (1986), section 4b:
// the middle of a shortest edit script splits the problem in two.
type search struct {
	a, b      []int
	fd, bd    []int // furthest x per diagonal x-y, forward and backward
	off       int   // index of diagonal 0 in fd and bd
	expensive int   // edit cost after which a split stops being optimal
	out       []pair
}

// compare appends the common subsequence of a[x0:x1] and b[y0:y1].
func (s *search) compare(x0, x1, y0, y1 int) {
	for x0 < x1 && y0 < y1 && s.a[x0] == s.b[y0] {
		s.out = append(s.out, pair{x0, y0})
		x0, y0 = x0+1, y0+1
	}
	suffix := 0
	for x1 > x0 && y1 > y0 && s.a[x1-1] == s.b[y1-1] {
		x1, y1 = x1-1, y1-1
		suffix++
	}
	if x0 < x1 && y0 < y1 {
		xm, ym := s.middle(x0, x1, y0, y1)
		s.compare(x0, xm, y0, ym)
		s.compare(xm, x1, ym, y1)
	}
	for k := range suffix {
		s.out = append(s.out, pair{x1 + k, y1 + k})
	}
}

// middle returns a point on a shortest path through a[x0:x1] and b[y0:y1]
// that leaves both halves smaller, extending the paths from both ends one
// edit at a time until they overlap. Past s.expensive edits it settles for
// the point that got furthest. Both sequences are non-empty and differ in
// their first and last elements.
func (s *search) middle(x0, x1, y0, y1 int) (xm, ym int) {
	fd, bd, o := s.fd, s.bd, s.off
	dmin, dmax := x0-y1, x1-y0 // the diagonals the box spans
	fmid, bmid := x0-y0, x1-y1 // where each path starts
	fmin, fmax, bmin, bmax := fmid, fmid, bmid, bmid
	odd := (fmid-bmid)&1 != 0
	fd[o+fmid], bd[o+bmid] = x0, x1
	for c := 1; ; c++ {
		// One more edit forward; unreachable diagonals hold sentinels.
		if fmin > dmin {
			fmin--
			fd[o+fmin-1] = -1
		} else {
			fmin++
		}
		if fmax < dmax {
			fmax++
			fd[o+fmax+1] = -1
		} else {
			fmax--
		}
		for d := fmax; d >= fmin; d -= 2 {
			x := fd[o+d+1] // an insertion
			if lo := fd[o+d-1]; lo >= x {
				x = lo + 1 // a deletion
			}
			y := x - d
			for x < x1 && y < y1 && s.a[x] == s.b[y] {
				x, y = x+1, y+1
			}
			fd[o+d] = x
			if odd && bmin <= d && d <= bmax && bd[o+d] <= x {
				return x, y
			}
		}

		// One more edit backward.
		if bmin > dmin {
			bmin--
			bd[o+bmin-1] = math.MaxInt
		} else {
			bmin++
		}
		if bmax < dmax {
			bmax++
			bd[o+bmax+1] = math.MaxInt
		} else {
			bmax--
		}
		for d := bmax; d >= bmin; d -= 2 {
			x := bd[o+d-1] // a deletion
			if hi := bd[o+d+1]; hi-1 < x {
				x = hi - 1 // an insertion
			}
			y := x - d
			for x > x0 && y > y0 && s.a[x-1] == s.b[y-1] {
				x, y = x-1, y-1
			}
			bd[o+d] = x
			if !odd && fmin <= d && d <= fmax && x <= fd[o+d] {
				return x, y
			}
		}

		if c >= s.expensive {
			if xm, ym, ok := s.furthest(x0, x1, y0, y1, fmin, fmax, bmin, bmax); ok {
				return xm, ym
			}
		}
	}
}

// furthest returns the end of whichever path, forward or backward, has
// covered more of the box, unless that point is a corner of the box and so
// would split nothing off.
func (s *search) furthest(x0, x1, y0, y1, fmin, fmax, bmin, bmax int) (xm, ym int, ok bool) {
	fd, bd, o := s.fd, s.bd, s.off
	fbest, fx := -1, 0
	for d := fmax; d >= fmin; d -= 2 {
		x := min(fd[o+d], x1)
		y := x - d
		if y > y1 {
			x, y = y1+d, y1
		}
		if x+y > fbest {
			fbest, fx = x+y, x
		}
	}
	bbest, bx := math.MaxInt, 0
	for d := bmax; d >= bmin; d -= 2 {
		x := max(x0, bd[o+d])
		y := x - d
		if y < y0 {
			x, y = y0+d, y0
		}
		if x+y < bbest {
			bbest, bx = x+y, x
		}
	}
	xm, ym = bx, bbest-bx
	if (x1+y1)-bbest < fbest-(x0+y0) {
		xm, ym = fx, fbest-fx
	}
	corner := (xm == x0 && ym == y0) || (xm == x1 && ym == y1)
	return xm, ym, !corner
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
