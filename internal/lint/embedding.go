package lint

import (
	"go/ast"
	"go/token"
	"go/types"

	"github.com/mvrahden/go-test/internal/gotestast"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ast/inspector"
)

// checkNoEmbedding reports each field of a suite or fixture the generator
// refuses: an embedded suite, fixture or type carrying a method gotest
// reads, and a field of suite type.
func checkNoEmbedding(pass *analysis.Pass, insp *inspector.Inspector, reached map[string]bool) {
	insp.Preorder([]ast.Node{(*ast.GenDecl)(nil)}, func(n ast.Node) {
		gd := n.(*ast.GenDecl)
		if gd.Tok != token.TYPE {
			return
		}
		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok || ts.Assign.IsValid() {
				continue
			}
			if _, ok := ts.Type.(*ast.StructType); !ok {
				continue
			}
			kind := declaredKind(pass, ts.Name.Name, reached)
			if kind == "" {
				continue
			}
			tn, ok := pass.TypesInfo.Defs[ts.Name].(*types.TypeName)
			if !ok {
				continue
			}
			st, ok := tn.Type().Underlying().(*types.Struct)
			if !ok {
				continue
			}
			for i := range st.NumFields() {
				field := st.Field(i)
				if err := gotestast.FieldShapeError(kind, ts.Name.Name, field, pass.Pkg); err != nil {
					report(pass, NoEmbedding, field.Pos(), "%s", err)
				}
			}
		}
	})
}
