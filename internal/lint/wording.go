package lint

import (
	"go/ast"
	"go/token"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ast/inspector"
)

// checkBehaviorWording flags a When or It description that opens with the word
// the spec already supplies. The renderer prefixes every When label with
// "when" and the ✓/✗ glyph plays the role of "it", so a description that
// starts with either says the word twice in the source and gains nothing in
// the spec. The fix drops the word and the separator after it; a description
// that is nothing but the word is left alone, because there is nothing to keep.
func checkBehaviorWording(pass *analysis.Pass, insp *inspector.Inspector) {
	insp.Preorder([]ast.Node{(*ast.CallExpr)(nil)}, func(n ast.Node) {
		call := n.(*ast.CallExpr)
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || len(call.Args) == 0 {
			return
		}
		var word, why string
		switch sel.Sel.Name {
		case "When":
			word, why = "when", "the spec supplies the connective; write the condition alone"
		case "It":
			word, why = "it", "the ✓ glyph already plays that role; write the behavior alone"
		default:
			return
		}
		if !namedPtrType(pass.TypesInfo.TypeOf(sel.X), gotestImportPath, "T") {
			return
		}
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING || len(lit.Value) < 2 {
			return
		}
		// Work on the source text between the quotes: the leading word is
		// plain ASCII either way, and keeping the rest byte-for-byte means the
		// fix never re-escapes what the developer wrote.
		quote, body := lit.Value[:1], lit.Value[1:len(lit.Value)-1]
		rest, ok := afterLeadingWord(body, word)
		if !ok {
			return
		}
		reportWithFix(pass, BehaviorWording, lit.Pos(),
			[]analysis.SuggestedFix{{
				Message: "drop the redundant word",
				TextEdits: []analysis.TextEdit{{
					Pos:     lit.Pos(),
					End:     lit.End(),
					NewText: []byte(quote + rest + quote),
				}},
			}},
			"%s description opens with %q — %s", sel.Sel.Name, word, why)
	})
}

// afterLeadingWord returns what follows word at the start of s when s opens
// with it as a whole word — case-insensitively, separated by a space, an
// underscore or a tab — and something follows. "whenever" does not open with
// "when", and "when" alone has nothing after it.
func afterLeadingWord(s, word string) (string, bool) {
	if len(s) <= len(word) || !strings.EqualFold(s[:len(word)], word) {
		return "", false
	}
	if c := s[len(word)]; c != ' ' && c != '_' && c != '\t' {
		return "", false
	}
	rest := strings.TrimLeft(s[len(word):], " _\t")
	return rest, rest != ""
}
