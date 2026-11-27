package mutate

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

// IgnoreDirective keeps the lines it names from being mutated. Written at
// the end of a line it names that line; on a line of its own, the
// declaration or statement starting on the next line, a whole function when
// it is one. Its reason is mandatory: one without a reason keeps nothing.
const IgnoreDirective = "//gotest:mutate-ignore"

// Ignore scopes.
const (
	ScopeLine      = "line"
	ScopeStatement = "statement"
	ScopeFunction  = "function"
)

// Ignore is one IgnoreDirective comment and the lines it keeps.
type Ignore struct {
	// Line is the comment's line; Start and End the lines it keeps.
	Line, Start, End int
	Scope            string
	// Func names the function a function scope keeps.
	Func   string
	Reason string
}

// Keeps reports whether the comment keeps line from being mutated.
func (ig *Ignore) Keeps(line int) bool {
	return ig.Reason != "" && ig.Start > 0 && ig.Start <= line && line <= ig.End
}

// Ignores lists the IgnoreDirective comments of a Go file's source, in
// line order. A file that does not parse has none.
func Ignores(path string, src []byte) []Ignore {
	if !strings.Contains(string(src), IgnoreDirective) {
		return nil
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil
	}
	lines := strings.Split(string(src), "\n")
	var out []Ignore
	for _, group := range f.Comments {
		for _, c := range group.List {
			reason, ok := directive(c.Text)
			if !ok {
				continue
			}
			pos := fset.Position(c.Pos())
			ig := Ignore{Line: pos.Line, Reason: reason}
			if before := lines[pos.Line-1][:pos.Column-1]; strings.TrimSpace(before) != "" {
				ig.Scope, ig.Start, ig.End = ScopeLine, pos.Line, pos.Line
			} else {
				ig.target(fset, f, fset.Position(group.End()).Line+1)
			}
			out = append(out, ig)
		}
	}
	return out
}

// directive reads a comment's reason; ok is false for another comment.
func directive(text string) (reason string, ok bool) {
	rest, found := strings.CutPrefix(text, IgnoreDirective)
	if !found || (rest != "" && rest[0] != ' ' && rest[0] != '\t') {
		return "", false
	}
	rest = strings.TrimSpace(rest)
	rest = strings.TrimSpace(strings.TrimPrefix(rest, "//"))
	return rest, true
}

// target gives ig the outermost declaration or statement starting on line,
// else that line alone.
func (ig *Ignore) target(fset *token.FileSet, f *ast.File, line int) {
	ast.Inspect(f, func(n ast.Node) bool {
		if n == nil || ig.Start > 0 {
			return false
		}
		start := fset.Position(n.Pos()).Line
		if fset.Position(n.End()).Line < line {
			return false // ends before the line: nothing inside starts on it
		}
		if start != line {
			return start < line
		}
		switch x := n.(type) {
		case *ast.FuncDecl:
			ig.Scope, ig.Func = ScopeFunction, funcDeclName(x)
		case ast.Decl, ast.Stmt:
			ig.Scope = ScopeStatement
		default:
			return true
		}
		ig.Start, ig.End = start, fset.Position(n.End()).Line
		return false
	})
	if ig.Start == 0 {
		ig.Scope, ig.Start, ig.End = ScopeLine, line, line
	}
}
