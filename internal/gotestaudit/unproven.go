package gotestaudit

import (
	"bufio"
	"cmp"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestspec"
)

// FileLines are the lines of one head file a change added or changed.
type FileLines struct {
	// Path is the file as the diff names it, slash-separated.
	Path string
	// Lines ascend; a file only deleted from has none.
	Lines []int
}

// ParseDiff reads the lines a unified diff adds to each new file, as git
// diff and diff -u write it. A deleted file is left out.
func ParseDiff(r io.Reader) ([]FileLines, error) {
	var out []FileLines
	var cur *FileLines
	oldLeft, newLeft, next := 0, 0, 0
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 16<<20)
	for sc.Scan() {
		line := sc.Text()
		if oldLeft > 0 || newLeft > 0 {
			switch {
			case strings.HasPrefix(line, "+"):
				if cur != nil {
					cur.Lines = append(cur.Lines, next)
				}
				next++
				newLeft--
			case strings.HasPrefix(line, "-"):
				oldLeft--
			case strings.HasPrefix(line, " "), line == "":
				next++
				oldLeft--
				newLeft--
			}
			continue
		}
		switch {
		case strings.HasPrefix(line, "+++ "):
			cur = nil
			name := diffPath(strings.TrimPrefix(line, "+++ "))
			if name == "" {
				continue
			}
			out = append(out, FileLines{Path: name})
			cur = &out[len(out)-1]
		case strings.HasPrefix(line, "@@ "):
			o, n, start, ok := hunkHeader(line)
			if !ok {
				return nil, fmt.Errorf("malformed hunk header %q", line)
			}
			oldLeft, newLeft, next = o, n, start
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		slices.Sort(out[i].Lines)
		out[i].Lines = slices.Compact(out[i].Lines)
	}
	return out, nil
}

// diffPath is the new file's path of a +++ line: its b/ prefix and a
// timestamp after a tab dropped, a quoted name unquoted; "" for /dev/null.
func diffPath(s string) string {
	if strings.HasPrefix(s, `"`) {
		if q, err := strconv.Unquote(s[:strings.LastIndex(s, `"`)+1]); err == nil {
			s = q
		}
	} else if i := strings.IndexByte(s, '\t'); i >= 0 {
		s = s[:i]
	}
	if s == "/dev/null" {
		return ""
	}
	return strings.TrimPrefix(s, "b/")
}

// hunkHeader reads "@@ -a[,b] +c[,d] @@": the old and new line counts and
// the first new line.
func hunkHeader(line string) (oldN, newN, start int, ok bool) {
	f := strings.Fields(line)
	if len(f) < 3 || !strings.HasPrefix(f[1], "-") || !strings.HasPrefix(f[2], "+") {
		return 0, 0, 0, false
	}
	_, oldN, ok1 := rangeOf(f[1][1:])
	start, newN, ok2 := rangeOf(f[2][1:])
	return oldN, newN, start, ok1 && ok2
}

func rangeOf(s string) (start, n int, ok bool) {
	a, b, has := strings.Cut(s, ",")
	start, err := strconv.Atoi(a)
	if err != nil {
		return 0, 0, false
	}
	n = 1
	if has {
		if n, err = strconv.Atoi(b); err != nil {
			return 0, 0, false
		}
	}
	return start, n, true
}

// ProdFile is a changed production file of the head.
type ProdFile struct {
	Module string
	// File is relative to its module's root, slash-separated.
	File    string
	Package string
	Lines   []int
	// Abs is the file's absolute path.
	Abs string
}

// key is the name a coverage profile gives the file.
func (f *ProdFile) key() string { return f.Package + "/" + path.Base(f.File) }

// Executable are the files of which a changed line lies in a function's
// body; a file that cannot be parsed counts.
func Executable(files []ProdFile) []ProdFile {
	var out []ProdFile
	for i := range files {
		if inFunction(&files[i]) {
			out = append(out, files[i])
		}
	}
	return out
}

// inFunction reports whether a changed line of f lies between the first
// statement's first line and the last statement's last line of a function
// or function literal.
func inFunction(f *ProdFile) bool {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, f.Abs, nil, parser.SkipObjectResolution)
	if err != nil {
		return true
	}
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		var body *ast.BlockStmt
		switch fn := n.(type) {
		case *ast.FuncDecl:
			body = fn.Body
		case *ast.FuncLit:
			body = fn.Body
		}
		if found || body == nil || len(body.List) == 0 {
			return !found
		}
		from, to := fset.Position(body.List[0].Pos()).Line, fset.Position(body.List[len(body.List)-1].End()).Line
		for _, l := range f.Lines {
			if from <= l && l <= to {
				found = true
			}
		}
		return !found
	})
	return found
}

// AddUnproven lists, per changed production file, the changed lines the
// profile's blocks know as executable and that no block covering them
// ran; a file of a package the profile covers nowhere has no coverage
// data. A line no block covers is not executable and not counted.
func AddUnproven(d *Document, files []ProdFile, blocks []gotestspec.CoverageBlock, cov Coverage) {
	byFile := map[string][]gotestspec.CoverageBlock{}
	profiled := map[string]bool{}
	for _, b := range blocks {
		byFile[b.File] = append(byFile[b.File], b)
		profiled[path.Dir(b.File)] = true
	}
	covered := map[string]bool{}
	s := &d.Summary
	for i := range files {
		f := &files[i]
		u := Unproven{Module: f.Module, File: f.File, Package: f.Package, Ranges: []LineRange{}}
		if !profiled[f.Package] {
			d.Unproven = append(d.Unproven, u)
			s.NoCoverage++
			continue
		}
		covered[f.Package] = true
		u.Coverage = true
		var unexecuted []int
		for _, l := range f.Lines {
			executable, ran := false, false
			for _, b := range byFile[f.key()] {
				if b.Stmts > 0 && b.StartLine <= l && l <= b.EndLine {
					executable = true
					ran = ran || b.Count > 0
				}
			}
			if !executable {
				continue
			}
			u.Changed++
			if !ran {
				unexecuted = append(unexecuted, l)
			}
		}
		s.ChangedLines += u.Changed
		if len(unexecuted) == 0 {
			continue
		}
		u.Unexecuted, u.Line, u.Ranges = len(unexecuted), unexecuted[0], ranges(unexecuted)
		s.Unproven += u.Unexecuted
		d.Unproven = append(d.Unproven, u)
	}
	slices.SortStableFunc(d.Unproven, func(a, b Unproven) int {
		return cmp.Or(cmp.Compare(a.Module, b.Module), cmp.Compare(a.File, b.File))
	})
	for p := range covered {
		cov.Packages = append(cov.Packages, p)
	}
	slices.Sort(cov.Packages)
	if cov.Packages == nil {
		cov.Packages = []string{}
	}
	d.Coverage = &cov
	setClass(s, ClassUnproven, len(d.Unproven))
	d.normalize()
}

// ranges folds ascending lines into runs.
func ranges(lines []int) []LineRange {
	var out []LineRange
	for _, l := range lines {
		if n := len(out); n > 0 && out[n-1].EndLine+1 == l {
			out[n-1].EndLine = l
			continue
		}
		out = append(out, LineRange{Line: l, EndLine: l})
	}
	return out
}
