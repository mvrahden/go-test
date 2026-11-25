package mutate

import (
	"path/filepath"
	"regexp"
	"strconv"
)

var errLine = regexp.MustCompile(`(?m)^(\S+\.go):(\d+):(\d+): `)

// Refused maps a compiler's errors to the mutants of the lines they name, a
// line without a mutant to the nearest mutant above it in its file: the
// schemata keep the source's lines. dir resolves a relative file name.
func Refused(out []byte, dir string, muts []Mutant) []int {
	var bad []int
	seen := map[int]bool{}
	add := func(id int) {
		if id != 0 && !seen[id] {
			seen[id] = true
			bad = append(bad, id)
		}
	}
	for _, m := range errLine.FindAllSubmatch(out, -1) {
		file := string(m[1])
		if !filepath.IsAbs(file) {
			file = filepath.Join(dir, file)
		}
		line, _ := strconv.Atoi(string(m[2]))
		hit, nearest, id := false, 0, 0
		for i := range muts {
			mu := &muts[i]
			if mu.File != file || mu.Line > line {
				continue
			}
			if mu.Line == line {
				add(mu.ID)
				hit = true
			} else if mu.Line >= nearest {
				nearest, id = mu.Line, mu.ID
			}
		}
		if !hit {
			add(id)
		}
	}
	return bad
}
