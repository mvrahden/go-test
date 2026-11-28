package gotestaudit

import (
	"crypto/sha256"
	"encoding/hex"
	"go/ast"
	"go/scanner"
	"go/token"
	"go/types"
	"os"
	"strings"
)

const gotestPath = "github.com/mvrahden/go-test/pkg/gotest"

// assertionFuncs are pkg/gotest's assertions: the functions that count for
// the test of the T they are given.
var assertionFuncs = map[string]bool{
	"Consistently": true, "Contains": true, "ElementsMatch": true, "Empty": true,
	"Equal": true, "Error": true, "ErrorAs": true, "ErrorContains": true,
	"ErrorIs": true, "Eventually": true, "Fail": true, "False": true,
	"Greater": true, "GreaterOrEqual": true, "InDelta": true, "JSONEq": true,
	"Len": true, "Less": true, "LessOrEqual": true, "MatchSnapshot": true,
	"Nil": true, "NoError": true, "NotContains": true, "NotEmpty": true,
	"NotEqual": true, "NotNil": true, "NotSame": true, "NotZero": true,
	"Panics": true, "Regexp": true, "Same": true, "Subset": true,
	"TimeIsNow": true, "TimeWithin": true, "True": true, "Zero": true,
}

// polls count once on their own T; what their callback asserts counts nothing.
var polls = map[string]bool{"Eventually": true, "Consistently": true}

// bodyReader reads bodies of one package's files.
type bodyReader struct {
	fset *token.FileSet
	info *types.Info
	src  map[string][]byte
}

// body hashes parts as one normalized body, skips left out, and counts the
// assertions, AssertsBy calls and calls that skip the test written in it.
func (r *bodyReader) body(parts, skips []ast.Node) (hash string, assertions int, assertsBy bool, skipCalls int) {
	h := sha256.New()
	for _, part := range parts {
		for _, tok := range r.tokens(part, skips) {
			h.Write([]byte(tok))
			h.Write([]byte{0})
		}
		assertions, assertsBy = r.count(part, skips, assertions, assertsBy)
		skipCalls += r.skipCalls(part, skips)
	}
	return hex.EncodeToString(h.Sum(nil))[:16], assertions, assertsBy, skipCalls
}

// tokens are node's tokens, comments, layout, semicolons and the skipped
// nodes left out: a behavior declared or dropped beside it is no change of
// its own.
func (r *bodyReader) tokens(node ast.Node, skips []ast.Node) []string {
	file := r.fset.File(node.Pos())
	if file == nil {
		return nil
	}
	src, ok := r.src[file.Name()]
	if !ok {
		src, _ = os.ReadFile(file.Name())
		r.src[file.Name()] = src
	}
	start, end := file.Offset(node.Pos()), file.Offset(node.End())
	if start < 0 || end > len(src) || start > end {
		return nil
	}
	type span struct{ from, to int }
	var spans []span
	for _, s := range skips {
		if r.fset.File(s.Pos()) == file && s.Pos() >= node.Pos() && s.End() <= node.End() {
			spans = append(spans, span{file.Offset(s.Pos()) - start, file.Offset(s.End()) - start})
		}
	}
	seg := src[start:end]
	fs := token.NewFileSet()
	var sc scanner.Scanner
	sc.Init(fs.AddFile("", -1, len(seg)), seg, nil, 0)
	var out []string
	inSpan := -1
	for {
		pos, tok, lit := sc.Scan()
		if tok == token.EOF {
			return out
		}
		off := int(pos) - 1
		if inSpan >= 0 && off < spans[inSpan].to {
			continue
		}
		inSpan = -1
		skipped := false
		for i, sp := range spans {
			if off >= sp.from && off < sp.to {
				inSpan, skipped = i, true
				break
			}
		}
		if skipped || tok == token.SEMICOLON {
			continue
		}
		if lit != "" {
			out = append(out, lit)
		} else {
			out = append(out, tok.String())
		}
	}
}

// count adds the assertions and AssertsBy calls node makes outside skips.
func (r *bodyReader) count(node ast.Node, skips []ast.Node, n int, by bool) (int, bool) {
	skip := map[ast.Node]bool{}
	for _, s := range skips {
		skip[s] = true
	}
	ast.Inspect(node, func(x ast.Node) bool {
		if x == nil || skip[x] {
			return false
		}
		call, ok := x.(*ast.CallExpr)
		if !ok {
			return true
		}
		fn := r.callee(call.Fun)
		if fn == nil || fn.Pkg() == nil || fn.Pkg().Path() != gotestPath {
			return true
		}
		sig, _ := fn.Type().(*types.Signature)
		switch {
		case sig != nil && sig.Recv() != nil:
			if fn.Name() == "AssertsBy" {
				by = true
			}
		case assertionFuncs[fn.Name()]:
			n++
			if polls[fn.Name()] {
				return false
			}
		}
		return true
	})
	return n, by
}

// skipCalls counts the calls node makes outside skips that skip the test:
// Skip, Skipf and SkipNow of package testing's types, Skipf of gotest's.
func (r *bodyReader) skipCalls(node ast.Node, skips []ast.Node) int {
	skip := map[ast.Node]bool{}
	for _, s := range skips {
		skip[s] = true
	}
	n := 0
	ast.Inspect(node, func(x ast.Node) bool {
		if x == nil || skip[x] {
			return false
		}
		call, ok := x.(*ast.CallExpr)
		if !ok {
			return true
		}
		fn := r.callee(call.Fun)
		if fn == nil || fn.Pkg() == nil {
			return true
		}
		if sig, _ := fn.Type().(*types.Signature); sig == nil || sig.Recv() == nil {
			return true
		}
		switch fn.Pkg().Path() {
		case "testing":
			if fn.Name() == "Skip" || fn.Name() == "Skipf" || fn.Name() == "SkipNow" {
				n++
			}
		case gotestPath:
			if fn.Name() == "Skipf" {
				n++
			}
		}
		return true
	})
	return n
}

// callee is the function a call's expression names, nil for any other.
func (r *bodyReader) callee(fun ast.Expr) *types.Func {
	if r.info == nil {
		return nil
	}
	for {
		switch f := fun.(type) {
		case *ast.IndexExpr:
			fun = f.X
			continue
		case *ast.IndexListExpr:
			fun = f.X
			continue
		case *ast.ParenExpr:
			fun = f.X
			continue
		case *ast.Ident:
			fn, _ := r.info.Uses[f].(*types.Func)
			return fn
		case *ast.SelectorExpr:
			fn, _ := r.info.Uses[f.Sel].(*types.Func)
			return fn
		}
		return nil
	}
}

// identity is a suite's or a method's name without its X_ or F_.
func identity(name string) string {
	if rest, ok := strings.CutPrefix(name, "X_"); ok {
		return rest
	}
	if rest, ok := strings.CutPrefix(name, "F_"); ok {
		return rest
	}
	return name
}
