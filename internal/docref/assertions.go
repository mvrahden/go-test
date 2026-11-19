package docref

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/doc"
	"go/parser"
	"go/printer"
	"go/token"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// Assertion is one exported assertion of pkg/gotest.
type Assertion struct {
	Name      string // "Equal"
	Signature string // "gotest.Equal[V any](t, expected, actual V)"
	Result    string // "" or the result type
	Doc       string // first sentence of the doc comment, without the name
	Group     string // "equality"
}

// testingInterface is the type of an assertion's first parameter.
const testingInterface = "testingT"

// groups orders the assertions for the reference: each group, then its
// names, in display order.
var groups = []struct {
	name  string
	names []string
}{
	{"unconditional", []string{"Fail"}},
	{"equality", []string{"Equal", "NotEqual", "Same", "NotSame"}},
	{"boolean", []string{"True", "False"}},
	{"zero", []string{"Zero", "NotZero", "Nil", "NotNil"}},
	{"collections", []string{"Empty", "NotEmpty", "Len", "Contains", "NotContains", "ElementsMatch", "Subset"}},
	{"errors", []string{"NoError", "Error", "ErrorIs", "ErrorAs", "ErrorContains"}},
	{"comparison", []string{"Greater", "GreaterOrEqual", "Less", "LessOrEqual"}},
	{"patterns", []string{"Regexp"}},
	{"numeric", []string{"InDelta"}},
	{"json", []string{"JSONEq"}},
	{"time", []string{"TimeWithin", "TimeIsNow"}},
	{"panics", []string{"Panics"}},
	{"snapshots", []string{"MatchSnapshot"}},
	{"polling", []string{"Eventually", "Consistently"}},
}

// Assertions reads them from the package directory, in the order of the
// groups; an assertion in no group comes last, with an empty Group.
func Assertions(pkgDir string) ([]Assertion, error) {
	fset, files, err := parsePackage(pkgDir)
	if err != nil {
		return nil, err
	}
	var as []Assertion
	for _, f := range files {
		for _, decl := range f.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && isAssertion(fn) {
				as = append(as, assertion(fset, fn))
			}
		}
	}
	rank := map[string]int{}
	for _, g := range groups {
		for _, name := range g.names {
			rank[name] = len(rank)
		}
	}
	slices.SortFunc(as, func(a, b Assertion) int {
		ra, oka := rank[a.Name]
		rb, okb := rank[b.Name]
		switch {
		case oka && okb:
			return ra - rb
		case oka != okb:
			if oka {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Name, b.Name)
	})
	return as, nil
}

// parsePackage parses the non-test Go files of dir, with their comments, in
// name order.
func parsePackage(dir string) (*token.FileSet, []*ast.File, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, err
	}
	fset := token.NewFileSet()
	var files []*ast.File
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			return nil, nil, err
		}
		files = append(files, f)
	}
	if len(files) == 0 {
		return nil, nil, fmt.Errorf("%s holds no Go file", dir)
	}
	return fset, files, nil
}

func isAssertion(fn *ast.FuncDecl) bool {
	if fn.Recv != nil || !fn.Name.IsExported() || len(fn.Type.Params.List) == 0 {
		return false
	}
	id, ok := fn.Type.Params.List[0].Type.(*ast.Ident)
	return ok && id.Name == testingInterface
}

func assertion(fset *token.FileSet, fn *ast.FuncDecl) Assertion {
	typeParams := map[string]bool{}
	var tparams []string
	if fn.Type.TypeParams != nil {
		for _, f := range fn.Type.TypeParams.List {
			for _, n := range f.Names {
				typeParams[n.Name] = true
			}
			tparams = append(tparams, field(fset, f, typeParams))
		}
	}
	params := []string{"t"}
	rest := fn.Type.Params.List[1:]
	if n := len(rest); n > 0 {
		if last := rest[n-1]; len(last.Names) == 1 && last.Names[0].Name == "msgAndArgs" {
			rest = rest[:n-1]
		}
	}
	for _, f := range rest {
		params = append(params, field(fset, f, typeParams))
	}
	sig := "gotest." + fn.Name.Name
	if len(tparams) > 0 {
		sig += "[" + strings.Join(tparams, ", ") + "]"
	}
	sig += "(" + strings.Join(params, ", ") + ")"
	var results []string
	if fn.Type.Results != nil {
		for _, f := range fn.Type.Results.List {
			results = append(results, field(fset, f, typeParams))
		}
	}
	result := strings.Join(results, ", ")
	if len(results) > 1 {
		result = "(" + result + ")"
	}
	return Assertion{
		Name:      fn.Name.Name,
		Signature: sig,
		Result:    result,
		Doc:       strings.TrimPrefix(synopsis(fn.Doc), fn.Name.Name+" "),
		Group:     groupOf(fn.Name.Name),
	}
}

// field prints a parameter field as the source writes it: its names, then
// its type, with the package's own types named as a caller writes them.
func field(fset *token.FileSet, f *ast.Field, typeParams map[string]bool) string {
	var b bytes.Buffer
	_ = printer.Fprint(&b, fset, f.Type)
	typ := qualify(b.String(), typeParams)
	if len(f.Names) == 0 {
		return typ
	}
	names := make([]string, 0, len(f.Names))
	for _, n := range f.Names {
		names = append(names, n.Name)
	}
	return strings.Join(names, ", ") + " " + typ
}

var exportedIdent = regexp.MustCompile(`\b[A-Z]\w*`)

// qualify prefixes "gotest." to each exported name of typ that is neither
// selected from another package nor a type parameter.
func qualify(typ string, typeParams map[string]bool) string {
	var b strings.Builder
	last := 0
	for _, m := range exportedIdent.FindAllStringIndex(typ, -1) {
		name := typ[m[0]:m[1]]
		if (m[0] > 0 && typ[m[0]-1] == '.') || typeParams[name] {
			continue
		}
		b.WriteString(typ[last:m[0]] + "gotest." + name)
		last = m[1]
	}
	b.WriteString(typ[last:])
	return b.String()
}

func synopsis(cg *ast.CommentGroup) string {
	if cg == nil {
		return ""
	}
	return new(doc.Package).Synopsis(cg.Text())
}

func groupOf(name string) string {
	for _, g := range groups {
		if slices.Contains(g.names, name) {
			return g.name
		}
	}
	return ""
}

// checkGroups fails on a grouped name the package does not export, and on a
// name in two groups.
func checkGroups(as []Assertion) error {
	var errs []error
	seen := map[string]string{}
	for _, g := range groups {
		for _, name := range g.names {
			if other, ok := seen[name]; ok {
				errs = append(errs, fmt.Errorf("%s is in groups %s and %s of internal/docref; keep it in one", name, other, g.name))
			}
			seen[name] = g.name
			if !slices.ContainsFunc(as, func(a Assertion) bool { return a.Name == name }) {
				errs = append(errs, fmt.Errorf("group %s of internal/docref names %s, which pkg/gotest does not export as an assertion; remove it", g.name, name))
			}
		}
	}
	return errors.Join(errs...)
}

// AssertionSections returns the renderers of assertions/all and assertions/<group>.
func AssertionSections(as []Assertion) map[string]Renderer {
	sections := map[string]Renderer{"assertions/all": assertionFormats(as)}
	for _, g := range groups {
		var members []Assertion
		for _, a := range as {
			if a.Group == g.name {
				members = append(members, a)
			}
		}
		sections["assertions/"+g.name] = assertionFormats(members)
	}
	return sections
}

func assertionFormats(as []Assertion) Renderer {
	full := func(a Assertion) string {
		if a.Result == "" {
			return a.Signature
		}
		return a.Signature + " " + a.Result
	}
	return formats(map[string]func() string{
		"go": func() string {
			width := 0
			for _, a := range as {
				width = max(width, len(full(a)))
			}
			var b strings.Builder
			b.WriteString("```go\n")
			for _, a := range as {
				fmt.Fprintf(&b, "%-*s  // %s\n", width, full(a), strings.TrimSuffix(a.Doc, "."))
			}
			b.WriteString("```\n")
			return b.String()
		},
		"table": func() string {
			var b strings.Builder
			b.WriteString("| Assertion | Checks |\n|---|---|\n")
			for _, a := range as {
				fmt.Fprintf(&b, "| `%s` | %s |\n", codeCell(full(a)), textCell(sentence(a.Doc)))
			}
			return b.String()
		},
		"html": func() string {
			var b strings.Builder
			for _, a := range as {
				fmt.Fprintf(&b, "<tr><td><code>%s</code></td><td>%s</td></tr>\n", html.EscapeString(full(a)), html.EscapeString(sentence(a.Doc)))
			}
			return b.String()
		},
	})
}
