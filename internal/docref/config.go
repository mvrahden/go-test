package docref

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"html"
	"slices"
	"strings"
)

// Field is a field of a config type, or a preset with the type it returns.
type Field struct{ Name, Type, Doc string }

// ConfigFields reads the fields of the named struct type of the package in
// pkgDir: name, type and the first sentence of the field's comment.
func ConfigFields(pkgDir, typeName string) ([]Field, error) {
	fset, files, err := parsePackage(pkgDir)
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				ts := spec.(*ast.TypeSpec)
				st, ok := ts.Type.(*ast.StructType)
				if !ok || ts.Name.Name != typeName {
					continue
				}
				var fields []Field
				for _, fd := range st.Fields.List {
					var b bytes.Buffer
					_ = printer.Fprint(&b, fset, fd.Type)
					for _, n := range fd.Names {
						fields = append(fields, Field{Name: n.Name, Type: b.String(), Doc: synopsis(fd.Doc)})
					}
				}
				return fields, nil
			}
		}
	}
	return nil, fmt.Errorf("%s is not a struct type of %s", typeName, pkgDir)
}

// configTypes are the types a preset returns.
var configTypes = []string{"SuiteConfig", "FixtureConfig"}

// Presets reads the functions of the package that take nothing and return a
// config type, in source order: name, the config type, the first sentence of
// the doc comment.
func Presets(pkgDir string) ([]Field, error) {
	_, files, err := parsePackage(pkgDir)
	if err != nil {
		return nil, err
	}
	var presets []Field
	for _, f := range files {
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || !fn.Name.IsExported() || len(fn.Type.Params.List) > 0 || fn.Type.Results == nil || len(fn.Type.Results.List) != 1 {
				continue
			}
			id, ok := fn.Type.Results.List[0].Type.(*ast.Ident)
			if !ok || !slices.Contains(configTypes, id.Name) {
				continue
			}
			presets = append(presets, Field{Name: fn.Name.Name, Type: id.Name, Doc: synopsis(fn.Doc)})
		}
	}
	return presets, nil
}

// ConfigSections returns the renderers of suiteconfig/fields,
// fixtureconfig/fields and config/presets.
func ConfigSections(pkgDir string) (map[string]Renderer, error) {
	suite, err := ConfigFields(pkgDir, "SuiteConfig")
	if err != nil {
		return nil, err
	}
	fixture, err := ConfigFields(pkgDir, "FixtureConfig")
	if err != nil {
		return nil, err
	}
	presets, err := Presets(pkgDir)
	if err != nil {
		return nil, err
	}
	for i := range presets {
		presets[i].Name += "()"
		presets[i].Type = "gotest." + presets[i].Type
	}
	return map[string]Renderer{
		"suiteconfig/fields":   fieldFormats("Field", suite),
		"fixtureconfig/fields": fieldFormats("Field", fixture),
		"config/presets":       fieldFormats("Preset", presets),
	}, nil
}

func fieldFormats(head string, fields []Field) Renderer {
	return formats(map[string]func() string{
		"table": func() string {
			var b strings.Builder
			fmt.Fprintf(&b, "| %s | Type | Meaning |\n|---|---|---|\n", head)
			for _, f := range fields {
				fmt.Fprintf(&b, "| `%s` | `%s` | %s |\n", codeCell(f.Name), codeCell(f.Type), textCell(f.Doc))
			}
			return b.String()
		},
		"html": func() string {
			var b strings.Builder
			for _, f := range fields {
				fmt.Fprintf(&b, "<tr><td><code>%s</code></td><td><code>%s</code></td><td>%s</td></tr>\n", html.EscapeString(f.Name), html.EscapeString(f.Type), html.EscapeString(f.Doc))
			}
			return b.String()
		},
		"list": func() string {
			names := make([]string, 0, len(fields))
			for _, f := range fields {
				names = append(names, "`"+f.Name+"`")
			}
			return strings.Join(names, ", ") + "\n"
		},
	})
}
