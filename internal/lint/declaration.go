package lint

import (
	"bytes"
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestast"
	"github.com/mvrahden/go-test/internal/protocol"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ast/inspector"
)

// checkSuiteDeclaration reports a suite or fixture declared inside a type
// block, which the generator refuses, with a fix that moves it into a
// declaration of its own right after the block.
func checkSuiteDeclaration(pass *analysis.Pass, insp *inspector.Inspector, reached map[string]bool) {
	insp.Preorder([]ast.Node{(*ast.GenDecl)(nil)}, func(n ast.Node) {
		gd := n.(*ast.GenDecl)
		if gd.Tok != token.TYPE || len(gd.Specs) == 1 {
			return
		}
		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			kind := declaredKind(pass, ts.Name.Name, reached)
			if kind == "" {
				continue
			}
			reportWithFix(pass, SuiteDeclaration, ts.Pos(), moveOutOfBlock(pass, gd, ts),
				"%s %s is declared inside a type block; give it its own type declaration", kind, ts.Name.Name)
		}
	})
}

// declaredKind is "suite" or "fixture" for a type the generator reads as
// one, or "". A package fixture counts only when a suite reaches it; a
// testify suite is no suite.
func declaredKind(pass *analysis.Pass, name string, reached map[string]bool) string {
	switch {
	case gotestast.IS_TEST_SUITE.MatchString(name):
		if isTestifySuite(pass, name) {
			return ""
		}
		return "suite"
	case strings.HasSuffix(name, protocol.SuffixSharedFixture) || reached[name]:
		return "fixture"
	}
	return ""
}

// reachedFixtures names the package fixtures the package's suites bind,
// directly or through other fixtures.
func reachedFixtures(pass *analysis.Pass) map[string]bool {
	var roots []*types.Struct
	scope := pass.Pkg.Scope()
	for _, name := range scope.Names() {
		tn, ok := scope.Lookup(name).(*types.TypeName)
		if !ok || tn.IsAlias() || !gotestast.IS_TEST_SUITE.MatchString(name) || isTestifySuite(pass, name) {
			continue
		}
		if st, ok := tn.Type().Underlying().(*types.Struct); ok {
			roots = append(roots, st)
		}
	}
	return gotestast.ReachedFixturesFrom(pass.Pkg.Path(), roots)
}

// moveOutOfBlock cuts ts, with its doc and line comments, out of the block
// and writes it as its own declaration after the block. A spec sharing a
// line with other code gets no fix.
func moveOutOfBlock(pass *analysis.Pass, gd *ast.GenDecl, ts *ast.TypeSpec) []analysis.SuggestedFix {
	tf := pass.Fset.File(ts.Pos())
	if tf == nil || !gd.Rparen.IsValid() {
		return nil
	}
	src, err := passSource(pass, tf.Name())
	if err != nil || tf.Size() != len(src) {
		return nil
	}
	start, end := ts.Pos(), ts.End()
	if ts.Doc != nil {
		start = ts.Doc.Pos()
	}
	if ts.Comment != nil {
		end = ts.Comment.End()
	}
	so, eo := tf.Offset(start), tf.Offset(end)
	lineStart := so
	for lineStart > 0 && (src[lineStart-1] == ' ' || src[lineStart-1] == '\t') {
		lineStart--
	}
	lineEnd := eo
	for lineEnd < len(src) && (src[lineEnd] == ' ' || src[lineEnd] == '\t' || src[lineEnd] == '\r') {
		lineEnd++
	}
	if lineStart > 0 && src[lineStart-1] != '\n' || lineEnd >= len(src) || src[lineEnd] != '\n' {
		return nil
	}
	nameAt := tf.Offset(ts.Pos()) - so
	text := string(src[so:so+nameAt]) + "type " + string(src[so+nameAt:eo])
	moved := bytes.ReplaceAll([]byte(text), []byte("\n\t"), []byte("\n"))
	return []analysis.SuggestedFix{{
		Message: "move " + ts.Name.Name + " into its own type declaration",
		TextEdits: []analysis.TextEdit{
			{Pos: tf.Pos(lineStart), End: tf.Pos(lineEnd + 1)},
			{Pos: gd.End(), End: gd.End(), NewText: append([]byte("\n\n"), moved...)},
		},
	}}
}
