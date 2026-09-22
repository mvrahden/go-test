package migrate

import (
	"fmt"
	"path/filepath"
	"strings"
)

// unifiedDiff renders the edit from before to after as one unified hunk per
// file, the shape a reader and `patch` both understand.
func unifiedDiff(path string, before, after []byte) string {
	a := strings.SplitAfter(string(before), "\n")
	b := strings.SplitAfter(string(after), "\n")
	if a[len(a)-1] == "" {
		a = a[:len(a)-1]
	}
	if b[len(b)-1] == "" {
		b = b[:len(b)-1]
	}
	name := filepath.ToSlash(path)
	var out strings.Builder
	fmt.Fprintf(&out, "--- a/%s\n+++ b/%s\n", name, name)
	fmt.Fprintf(&out, "@@ -1,%d +1,%d @@\n", len(a), len(b))
	for _, op := range lcsEdits(a, b) {
		out.WriteString(op)
	}
	return out.String()
}

// lcsEdits walks a longest-common-subsequence table back to the edit script,
// one prefixed line per entry.
func lcsEdits(a, b []string) []string {
	n, m := len(a), len(b)
	table := make([][]int32, n+1)
	for i := range table {
		table[i] = make([]int32, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				table[i][j] = table[i+1][j+1] + 1
			} else {
				table[i][j] = max(table[i+1][j], table[i][j+1])
			}
		}
	}
	var edits []string
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			edits = append(edits, " "+withNewline(a[i]))
			i++
			j++
		case table[i+1][j] >= table[i][j+1]:
			edits = append(edits, "-"+withNewline(a[i]))
			i++
		default:
			edits = append(edits, "+"+withNewline(b[j]))
			j++
		}
	}
	for ; i < n; i++ {
		edits = append(edits, "-"+withNewline(a[i]))
	}
	for ; j < m; j++ {
		edits = append(edits, "+"+withNewline(b[j]))
	}
	return edits
}

func withNewline(line string) string {
	if strings.HasSuffix(line, "\n") {
		return line
	}
	return line + "\n"
}
