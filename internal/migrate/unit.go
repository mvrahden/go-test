package migrate

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// unitFile is one file of a unit. Test files are migrated; the others only
// take part in resolving names.
type unitFile struct {
	path    string
	src     []byte
	f       *ast.File
	test    bool
	refused bool
	marks   []mark // refusal markers, placed on src
	out     []byte // migrated source; nil while unchanged
}

// mark is a TODO(gotest-migrate) marker due above pos.
type mark struct {
	pos token.Pos
	msg string
}

// unit is the test code of one directory, migrated together: the package's
// own test files and its external _test package, which may use the
// former's suites. A suite's methods, runner and references are found in
// whichever of its files declares them.
type unit struct {
	importPath string
	fset       *token.FileSet
	files      []*unitFile
	info       *types.Info
	pkgs       map[string]*types.Package // by package clause
	suites     []*suiteDecl              // in declaration order
	byObj      map[types.Object]*suiteDecl
}

// suiteDecl is a struct type embedding suite.Suite.
type suiteDecl struct {
	obj     *types.TypeName
	spec    *ast.TypeSpec
	file    *unitFile
	newName string
	// refusal says why the suite is left as it is; empty when it migrates.
	refusal string
	// marked: refusal is the text of a marker due at the suite's type when
	// its file carries none of its own.
	marked bool
}

func (s *suiteDecl) migrated() bool { return s.refusal == "" }

// errUnresolved stands in for every import. Names declared in the unit
// resolve without them, and that is all the migration needs.
var errUnresolved = errors.New("not loaded")

type importerFunc func(path string) (*types.Package, error)

func (f importerFunc) Import(path string) (*types.Package, error) { return f(path) }

// loadUnit parses tests and context and resolves names across them.
func loadUnit(importPath string, tests, context []string) (*unit, error) {
	u := &unit{importPath: importPath, fset: token.NewFileSet()}
	for _, path := range tests {
		src, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		f, err := parser.ParseFile(u.fset, path, src, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		u.files = append(u.files, &unitFile{path: path, src: src, f: f, test: true})
	}
	for _, path := range context {
		src, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		// A context file that does not parse resolves nothing; it is not
		// the migration's to fix.
		f, err := parser.ParseFile(u.fset, path, src, parser.SkipObjectResolution)
		if err != nil {
			continue
		}
		u.files = append(u.files, &unitFile{path: path, src: src, f: f})
	}
	u.check()
	return u, nil
}

// check type-checks the unit's packages, the package itself first so its
// external _test package can import it.
func (u *unit) check() {
	u.info = &types.Info{
		Types:      map[ast.Expr]types.TypeAndValue{},
		Defs:       map[*ast.Ident]types.Object{},
		Uses:       map[*ast.Ident]types.Object{},
		Implicits:  map[ast.Node]types.Object{},
		Selections: map[*ast.SelectorExpr]*types.Selection{},
	}
	byPkg := map[string][]*ast.File{}
	var names []string
	for _, uf := range u.files {
		name := uf.f.Name.Name
		if byPkg[name] == nil {
			names = append(names, name)
		}
		byPkg[name] = append(byPkg[name], uf.f)
	}
	sort.SliceStable(names, func(i, j int) bool {
		return !strings.HasSuffix(names[i], "_test") && strings.HasSuffix(names[j], "_test")
	})
	u.pkgs = map[string]*types.Package{}
	var own *types.Package
	for _, name := range names {
		path := u.importPath
		if path == "" {
			path = name
		} else if strings.HasSuffix(name, "_test") {
			path += "_test"
		}
		conf := types.Config{
			Importer: importerFunc(func(p string) (*types.Package, error) {
				if own != nil && u.importPath != "" && p == u.importPath {
					return own, nil
				}
				return nil, errUnresolved
			}),
			Error: func(error) {},
		}
		pkg, _ := conf.Check(path, u.fset, byPkg[name], u.info)
		u.pkgs[name] = pkg
		if own == nil && !strings.HasSuffix(name, "_test") {
			own = pkg
		}
	}
}

// fileAt is the unit file holding pos.
func (u *unit) fileAt(pos token.Pos) *unitFile {
	for _, uf := range u.files {
		if uf.f.FileStart <= pos && pos <= uf.f.FileEnd {
			return uf
		}
	}
	return nil
}

// isPkg reports whether ident names the import of path.
func (u *unit) isPkg(ident *ast.Ident, paths ...string) bool {
	pn, ok := u.info.Uses[ident].(*types.PkgName)
	if !ok {
		return false
	}
	for _, p := range paths {
		if pn.Imported().Path() == p {
			return true
		}
	}
	return false
}

// isSuiteField reports whether field embeds suite.Suite.
func (u *unit) isSuiteField(field *ast.Field) bool {
	if len(field.Names) != 0 {
		return false
	}
	sel, ok := field.Type.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Suite" {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	return ok && u.isPkg(ident, suitePkg)
}

// suiteOf is the suite a type expression names, nil for any other.
func (u *unit) suiteOf(expr ast.Expr) *suiteDecl {
	for {
		switch x := expr.(type) {
		case *ast.StarExpr:
			expr = x.X
			continue
		case *ast.ParenExpr:
			expr = x.X
			continue
		case *ast.Ident:
			return u.byObj[u.info.Uses[x]]
		case *ast.SelectorExpr:
			return u.byObj[u.info.Uses[x.Sel]]
		}
		return nil
	}
}

// suiteOfType is the suite t or *t is, nil for any other.
func (u *unit) suiteOfType(t types.Type) *suiteDecl {
	if t == nil {
		return nil
	}
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	n, ok := t.(*types.Named)
	if !ok {
		return nil
	}
	return u.byObj[n.Obj()]
}

// findSuites collects every struct type a test file declares with
// suite.Suite embedded. A generic one is not migrated.
func (u *unit) findSuites() {
	u.byObj = map[types.Object]*suiteDecl{}
	for _, uf := range u.files {
		if !uf.test {
			continue
		}
		for _, decl := range uf.f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok || ts.TypeParams != nil {
					continue
				}
				st, ok := ts.Type.(*ast.StructType)
				if !ok || st.Fields == nil {
					continue
				}
				obj, _ := u.info.Defs[ts.Name].(*types.TypeName)
				if obj == nil {
					continue
				}
				for _, field := range st.Fields.List {
					if u.isSuiteField(field) {
						s := &suiteDecl{obj: obj, spec: ts, file: uf, newName: DeriveNewName(obj.Name())}
						u.suites = append(u.suites, s)
						u.byObj[obj] = s
						break
					}
				}
			}
		}
	}
}

// refuse decides which suites stay as they are. A suite another struct
// embeds would strand the child when renamed, so it is refused, with the
// file declaring it and the file declaring the child; so is a suite whose
// new name is taken. A refused file is left whole, so every suite it
// declares or refers to is refused too, until nothing changes.
func (u *unit) refuse() {
	refused := map[*unitFile]bool{}
	for _, uf := range u.files {
		if !uf.test {
			continue
		}
		ast.PreorderStack(uf.f, nil, func(n ast.Node, stack []ast.Node) bool {
			st, ok := n.(*ast.StructType)
			if !ok || st.Fields == nil {
				return true
			}
			for _, field := range st.Fields.List {
				if len(field.Names) != 0 {
					continue
				}
				base := u.suiteOf(field.Type)
				if base == nil {
					continue
				}
				child, pos := "a struct", field.Pos()
				for i := len(stack) - 1; i >= 0; i-- {
					if ts, ok := stack[i].(*ast.TypeSpec); ok {
						child, pos = ts.Name.Name, ts.Pos()
						if gd, ok := stack[max(i-1, 0)].(*ast.GenDecl); ok && !gd.Lparen.IsValid() {
							pos = gd.Pos()
						}
						break
					}
				}
				uf.marks = append(uf.marks, mark{pos: pos, msg: child + " embeds " + base.obj.Name() + ", not suite.Suite — migrate this file by hand"})
				if base.migrated() {
					base.refusal = base.obj.Name() + " is embedded by " + child + " in " + filepath.Base(uf.path) + " — migrate this file by hand"
					base.marked = true
				}
				refused[uf] = true
				refused[base.file] = true
			}
			return true
		})
	}

	taken := map[string]*suiteDecl{}
	for _, s := range u.suites {
		other := taken[s.newName]
		clash := other != nil && other.obj.Pkg() == s.obj.Pkg()
		if s.newName != s.obj.Name() && s.obj.Pkg() != nil && s.obj.Pkg().Scope().Lookup(s.newName) != nil {
			clash = true
		}
		taken[s.newName] = s
		if clash && s.migrated() {
			s.refusal = "renaming " + s.obj.Name() + " to " + s.newName + " collides with another declaration — migrate this file by hand"
			s.file.marks = append(s.file.marks, mark{pos: s.spec.Pos(), msg: s.refusal})
			refused[s.file] = true
		}
	}

	users := map[*suiteDecl]map[*unitFile]bool{}
	for ident, obj := range u.info.Uses {
		s := u.byObj[obj]
		if s == nil {
			continue
		}
		if users[s] == nil {
			users[s] = map[*unitFile]bool{}
		}
		if uf := u.fileAt(ident.Pos()); uf != nil {
			users[s][uf] = true
		}
	}
	for changed := true; changed; {
		changed = false
		for _, s := range u.suites {
			if !s.migrated() {
				continue
			}
			if refused[s.file] {
				s.refusal = "declared in a file left for hand migration"
			}
			for _, uf := range u.files {
				if users[s][uf] && refused[uf] && s.migrated() {
					s.refusal = s.obj.Name() + " is used in " + filepath.Base(uf.path) + ", which is left for hand migration — migrate this file by hand"
					s.marked = true
				}
			}
			if !s.migrated() {
				refused[s.file] = true
				changed = true
			}
		}
	}

	// A refused file with no marker of its own says which suite holds it.
	for uf := range refused {
		uf.refused = true
		if len(uf.marks) > 0 {
			continue
		}
		for _, s := range u.suites {
			if s.file == uf && s.marked {
				uf.marks = append(uf.marks, mark{pos: s.spec.Pos(), msg: s.refusal})
			}
		}
	}
}

// migrate transforms every test file of the unit that is not refused,
// then marks what the migration could not convert in the result.
func (u *unit) migrate() error {
	u.findSuites()
	if len(u.suites) == 0 {
		return nil
	}
	u.refuse()

	migrated := false
	for _, s := range u.suites {
		migrated = migrated || s.migrated()
	}
	for _, uf := range u.files {
		if uf.refused {
			anns := make([]todoAnnotation, 0, len(uf.marks))
			for _, m := range uf.marks {
				anns = append(anns, todoAnnotation{line: u.fset.Position(m.pos).Line, msg: m.msg})
			}
			out, err := insertMarkers(uf.src, anns)
			if err != nil {
				return fmt.Errorf("annotate %s: %w", uf.path, err)
			}
			uf.out = out
			continue
		}
		if !uf.test || !migrated {
			continue
		}
		if !u.transformFile(uf.f) {
			continue
		}
		var buf bytes.Buffer
		if err := format.Node(&buf, u.fset, uf.f); err != nil {
			return fmt.Errorf("format %s: %w", uf.path, err)
		}
		out, err := format.Source(buf.Bytes())
		if err != nil {
			return fmt.Errorf("gofmt %s: %w", uf.path, err)
		}
		uf.out = out
	}
	if !migrated {
		return nil
	}
	return u.annotate()
}

// annotate re-reads the migrated unit and marks every construct in it the
// migration could not convert, so nothing is left silently broken.
func (u *unit) annotate() error {
	contents := map[string][]byte{}
	for _, uf := range u.files {
		contents[uf.path] = uf.src
		if uf.out != nil {
			contents[uf.path] = uf.out
		}
	}
	after := &unit{importPath: u.importPath, fset: token.NewFileSet()}
	for _, uf := range u.files {
		f, err := parser.ParseFile(after.fset, uf.path, contents[uf.path], parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			return fmt.Errorf("annotate %s: %w", uf.path, err)
		}
		after.files = append(after.files, &unitFile{path: uf.path, src: contents[uf.path], f: f, test: uf.test})
	}
	after.check()

	// The migrated suites, under their new names.
	after.byObj = map[types.Object]*suiteDecl{}
	for _, s := range u.suites {
		if !s.migrated() {
			continue
		}
		pkg := after.pkgs[s.file.f.Name.Name]
		if pkg == nil {
			continue
		}
		if obj, ok := pkg.Scope().Lookup(s.newName).(*types.TypeName); ok {
			after.byObj[obj] = s
		}
	}

	for i, uf := range u.files {
		if !uf.test || uf.refused {
			continue
		}
		anns := after.unconverted(after.files[i].f)
		if len(anns) == 0 {
			continue
		}
		out, err := insertMarkers(contents[uf.path], anns)
		if err != nil {
			return fmt.Errorf("annotate %s: %w", uf.path, err)
		}
		uf.out = out
	}
	return nil
}
