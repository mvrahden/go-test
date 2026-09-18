package lint

import (
	"go/ast"
	"go/types"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestast"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ast/inspector"
)

// checkSuiteConfigPartial flags a SuiteConfig method whose config starts from
// a gotest.SuiteConfig literal that leaves Timeout or SetupTimeout unset. The
// literal replaces DefaultSuiteConfig wholesale, so an unset timeout is not
// the 30-second default but no deadline at all: a hung method or BeforeAll
// then holds the run until go test's own -timeout fires. The fix composes
// the same fields onto DefaultSuiteConfig(), the form the generator reads too.
func checkSuiteConfigPartial(pass *analysis.Pass, insp *inspector.Inspector, suites map[string]*suiteInfo) {
	insp.Preorder([]ast.Node{(*ast.FuncDecl)(nil)}, func(n ast.Node) {
		fd := n.(*ast.FuncDecl)
		if fd.Recv == nil || fd.Name.Name != "SuiteConfig" {
			return
		}
		if _, ok := suites[receiverTypeName(fd.Recv)]; !ok {
			return
		}
		// A body the generator rejects is the generator's to report.
		body, err := gotestast.ParseSuiteConfigBody(pass.TypesInfo, fd)
		if err != nil || body.Literal == nil || !namedType(pass.TypesInfo.TypeOf(body.Literal), gotestImportPath, "SuiteConfig") {
			return
		}
		set := map[string]bool{}
		for _, kv := range body.Fields {
			key, ok := kv.Key.(*ast.Ident)
			if !ok {
				return
			}
			set[key.Name] = true
		}
		// A field assigned onto the literal afterwards is set all the same.
		for _, name := range body.AssignedFields() {
			set[name] = true
		}
		var missing []string
		for _, name := range []string{"Timeout", "SetupTimeout"} {
			if !set[name] {
				missing = append(missing, name)
			}
		}
		if len(missing) == 0 {
			return
		}

		var fixes []analysis.SuggestedFix
		if text, ok := composedConfig(pass, fd, body); ok {
			fixes = []analysis.SuggestedFix{{
				Message: "compose the config from DefaultSuiteConfig()",
				TextEdits: []analysis.TextEdit{{
					Pos:     body.Holder.Pos(),
					End:     body.Holder.End(),
					NewText: []byte(text),
				}},
			}}
		}
		reportWithFix(pass, SuiteConfigPartial, body.Literal.Pos(), fixes,
			"SuiteConfig literal leaves %s unset — the literal replaces the defaults wholesale, so an unset timeout is no deadline; compose from DefaultSuiteConfig()",
			strings.Join(missing, " and "))
	})
}

// composedConfig renders the compose form that replaces the statement holding
// the literal: `cfg := gotest.DefaultSuiteConfig()`, one assignment per field
// the literal set, and — when the literal was returned directly — `return
// cfg`. It declines when the file cannot name the gotest package or when the
// receiver already occupies the name the fix would introduce.
func composedConfig(pass *analysis.Pass, fd *ast.FuncDecl, body *gotestast.SuiteConfigBody) (string, bool) {
	qual, ok := gotestQualifier(pass, body.Literal.Pos())
	if !ok {
		return "", false
	}
	cfgName := body.CfgName
	if body.Returned() {
		cfgName = "cfg"
		for _, name := range fd.Recv.List[0].Names {
			if name.Name == cfgName {
				return "", false
			}
		}
		if len(body.Fields) == 0 {
			return "return " + qual + "DefaultSuiteConfig()", true
		}
	}
	read := func(name string) []byte {
		content, _ := pass.ReadFile(name)
		return content
	}
	indent := sourceIndent(pass.Fset, read, body.Holder.Pos())
	var b strings.Builder
	b.WriteString(cfgName + " := " + qual + "DefaultSuiteConfig()")
	for _, kv := range body.Fields {
		b.WriteString("\n" + indent + cfgName + "." + kv.Key.(*ast.Ident).Name + " = " + renderExpr(pass.Fset, kv.Value))
	}
	if body.Returned() {
		b.WriteString("\n" + indent + "return " + cfgName)
	}
	return b.String(), true
}

// namedType reports whether t is pkgPath.name, looking through aliases.
func namedType(t types.Type, pkgPath, name string) bool {
	named, ok := types.Unalias(t).(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	return obj.Name() == name && obj.Pkg() != nil && obj.Pkg().Path() == pkgPath
}
