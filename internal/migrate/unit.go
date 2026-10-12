package migrate

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"go/types"
	"path/filepath"
	"strings"
)

// unitFile is one test file of a unit.
type unitFile struct {
	path    string
	src     []byte
	f       *ast.File
	refused bool
	marks   []mark // refusal markers, placed on src
	out     []byte // migrated source; nil while unchanged
	// unchecked names what keeps the file out of every configuration
	// migrate type-checks; such a file has syntax but no types.
	unchecked string
	// gate is a //go:build expression the migration moves onto the file.
	gate string
}

// mark is a TODO(gotest-migrate) marker due above pos.
type mark struct {
	pos token.Pos
	msg string
}

// unit is the test code of one directory, migrated together: the package's
// own test files and its external _test package, which may use the
// former's suites, under every build configuration they need. A suite's
// methods, runner and references are found in whichever of its files
// declares them.
type unit struct {
	fset    *token.FileSet
	files   []*unitFile
	info    *types.Info               // merged across configurations
	scopes  map[string][]*types.Scope // package scopes by path, one per configuration
	filePkg map[*ast.File]*types.Package
	suites  []*suiteDecl // in declaration order
	byKey   map[string]*suiteDecl
	// hooks maps a suite method gotest calls with t, by methodKey, to
	// its gotest name.
	hooks map[string]string
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
	// note is a marker due at the migrated suite's type.
	note string
}

func (s *suiteDecl) migrated() bool { return s.refusal == "" }

// objKey names a package-level object the same in every configuration;
// the configurations type-check apart, so their objects differ.
func objKey(obj types.Object) string {
	if obj == nil || obj.Pkg() == nil || obj.Parent() != obj.Pkg().Scope() {
		return ""
	}
	return obj.Pkg().Path() + "." + obj.Name()
}

// methodKey names a method by its receiver's type and its name.
func methodKey(fn *types.Func) string {
	sig, ok := fn.Type().(*types.Signature)
	if !ok || sig.Recv() == nil {
		return ""
	}
	t := sig.Recv().Type()
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	n, ok := t.(*types.Named)
	if !ok {
		return ""
	}
	return objKey(n.Obj()) + "." + fn.Name()
}

// suiteFor is the suite obj declares, nil for any other object.
func (u *unit) suiteFor(obj types.Object) *suiteDecl {
	if k := objKey(obj); k != "" {
		return u.byKey[k]
	}
	return nil
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

// isSuiteField reports whether field embeds suite.Suite or *suite.Suite.
func (u *unit) isSuiteField(field *ast.Field) bool {
	if len(field.Names) != 0 {
		return false
	}
	typ := field.Type
	if star, ok := typ.(*ast.StarExpr); ok {
		typ = star.X
	}
	sel, ok := typ.(*ast.SelectorExpr)
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
			return u.suiteFor(u.info.Uses[x])
		case *ast.SelectorExpr:
			return u.suiteFor(u.info.Uses[x.Sel])
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
	return u.suiteFor(n.Obj())
}

// embedsTestifySuite reports whether t embeds suite.Suite, at any depth.
func embedsTestifySuite(t types.Type, seen map[types.Type]bool) bool {
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	if n, ok := t.(*types.Named); ok {
		if n.Obj().Pkg() != nil && n.Obj().Pkg().Path() == suitePkg && n.Obj().Name() == "Suite" {
			return true
		}
	}
	if seen[t] {
		return false
	}
	seen[t] = true
	st, ok := t.Underlying().(*types.Struct)
	if !ok {
		return false
	}
	for i := range st.NumFields() {
		if f := st.Field(i); f.Embedded() && embedsTestifySuite(f.Type(), seen) {
			return true
		}
	}
	return false
}

// findSuites collects every struct type a test file declares with
// suite.Suite embedded. A testify suite it cannot convert is marked and
// its file refused: a generic one, or one that inherits suite.Suite from
// a type of another package.
func (u *unit) findSuites() map[*unitFile]bool {
	u.byKey = map[string]*suiteDecl{}
	refused := map[*unitFile]bool{}
	for _, uf := range u.files {
		if uf.unchecked != "" {
			continue
		}
		for _, decl := range uf.f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
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
				direct := false
				for _, field := range st.Fields.List {
					direct = direct || u.isSuiteField(field)
				}
				switch {
				case direct && ts.TypeParams != nil:
					uf.marks = append(uf.marks, mark{pos: specPos(gd, ts), msg: obj.Name() + " is a generic testify suite; gotest instantiates generic suites through type aliases — migrate it by hand"})
					refused[uf] = true
				case direct:
					s := &suiteDecl{obj: obj, spec: ts, file: uf, newName: DeriveNewName(obj.Name())}
					u.suites = append(u.suites, s)
					u.byKey[objKey(obj)] = s
				default:
					if base := u.foreignBase(st); base != "" {
						uf.marks = append(uf.marks, mark{pos: specPos(gd, ts), msg: obj.Name() + " embeds " + base + ", a testify suite of another package; gotest suites do not inherit hooks or assertions — migrate both by hand"})
						refused[uf] = true
					}
				}
			}
		}
	}
	return refused
}

// foreignBase names the embedded type of another package st inherits
// suite.Suite through, empty for none.
func (u *unit) foreignBase(st *ast.StructType) string {
	for _, field := range st.Fields.List {
		if len(field.Names) != 0 {
			continue
		}
		t := u.info.TypeOf(field.Type)
		if t == nil {
			continue
		}
		base := t
		if p, ok := base.(*types.Pointer); ok {
			base = p.Elem()
		}
		n, ok := base.(*types.Named)
		if !ok || n.Obj().Pkg() == nil || u.scopes[n.Obj().Pkg().Path()] != nil {
			continue // declared here: refuse handles a suite embedding a suite
		}
		if embedsTestifySuite(t, map[types.Type]bool{}) {
			return n.Obj().Pkg().Name() + "." + n.Obj().Name()
		}
	}
	return ""
}

// specPos is where a marker for ts goes: above its own line, or above the
// declaration when it is not grouped.
func specPos(gd *ast.GenDecl, ts *ast.TypeSpec) token.Pos {
	if !gd.Lparen.IsValid() {
		return gd.Pos()
	}
	return ts.Pos()
}

// refuse decides which suites stay as they are. A suite another struct
// embeds would strand the child when renamed, so it is refused, with the
// file declaring it and the file declaring the child; so is a suite whose
// new name is taken, and one a file migrate cannot type-check refers to. A
// refused file is left whole, so every suite it declares or refers to is
// refused too, until nothing changes.
func (u *unit) refuse(refused map[*unitFile]bool) {
	for _, uf := range u.files {
		if uf.unchecked != "" {
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
						if gd, ok := stack[max(i-1, 0)].(*ast.GenDecl); ok {
							pos = specPos(gd, ts)
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

	// A suite keeping its name holds it; one renamed onto a name taken,
	// or still unexported, is refused.
	taken := map[string]bool{}
	for _, s := range u.suites {
		if s.newName == s.obj.Name() {
			taken[s.obj.Pkg().Path()+"."+s.newName] = true
		}
	}
	for _, s := range u.suites {
		if s.newName == s.obj.Name() || !s.migrated() {
			continue
		}
		key := s.obj.Pkg().Path() + "." + s.newName
		clash := taken[key]
		for _, scope := range u.scopes[s.obj.Pkg().Path()] {
			clash = clash || scope.Lookup(s.newName) != nil
		}
		taken[key] = true
		switch {
		case !token.IsExported(s.newName):
			s.refusal = s.obj.Name() + " cannot take an exported name, and gotest runs only exported suites — rename it and migrate this file by hand"
		case clash:
			s.refusal = "renaming " + s.obj.Name() + " to " + s.newName + " collides with another declaration — migrate this file by hand"
		default:
			continue
		}
		s.file.marks = append(s.file.marks, mark{pos: s.spec.Pos(), msg: s.refusal})
		refused[s.file] = true
	}

	users := map[*suiteDecl]map[*unitFile]bool{}
	use := func(s *suiteDecl, uf *unitFile) {
		if users[s] == nil {
			users[s] = map[*unitFile]bool{}
		}
		users[s][uf] = true
	}
	for ident, obj := range u.info.Uses {
		if s := u.suiteFor(obj); s != nil {
			if uf := u.fileAt(ident.Pos()); uf != nil {
				use(s, uf)
			}
		}
	}
	// A file without types refers to a suite by its name, and is left as
	// it is with a marker when it does or when it uses testify.
	for _, uf := range u.files {
		if uf.unchecked == "" {
			continue
		}
		mentions := false
		ast.Inspect(uf.f, func(n ast.Node) bool {
			if ident, ok := n.(*ast.Ident); ok {
				for _, s := range u.suites {
					if ident.Name == s.obj.Name() {
						use(s, uf)
						mentions = true
					}
				}
			}
			return true
		})
		if mentions || importsTestify(uf.f) {
			uf.marks = append(uf.marks, mark{pos: uf.f.Package, msg: "migrate cannot type-check this file: " + uf.unchecked + " keeps it out of every build it checked — migrate it by hand"})
			refused[uf] = true
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

// importsTestify reports whether f imports a testify package.
func importsTestify(f *ast.File) bool {
	for _, is := range f.Imports {
		if strings.HasPrefix(strings.Trim(is.Path.Value, `"`), "github.com/stretchr/testify/") {
			return true
		}
	}
	return false
}

// migrate transforms every test file of the unit that is not refused and
// leaves a refused one with its markers. What the result still needs is
// marked later, from its own type-check.
func (u *unit) migrate() error {
	refused := u.findSuites()
	u.refuse(refused)

	migrated := false
	for _, s := range u.suites {
		migrated = migrated || s.migrated()
	}
	if migrated {
		u.collectHooks()
		u.gate()
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
		if !migrated || uf.unchecked != "" {
			continue
		}
		if !u.transformFile(uf.f) && uf.gate == "" {
			continue
		}
		var buf bytes.Buffer
		if uf.gate != "" {
			buf.WriteString("//go:build " + uf.gate + "\n\n")
		}
		if err := format.Node(&buf, u.fset, uf.f); err != nil {
			return fmt.Errorf("format %s: %w", uf.path, err)
		}
		out, err := formatImports(uf.path, buf.Bytes())
		if err != nil {
			return fmt.Errorf("gofmt %s: %w", uf.path, err)
		}
		uf.out = out
	}
	return nil
}

// collectHooks records the methods of migrated suites that gain t: their
// Test methods and the hooks gotest renames.
func (u *unit) collectHooks() {
	u.hooks = map[string]string{}
	for _, uf := range u.files {
		if uf.refused || uf.unchecked != "" {
			continue
		}
		for _, decl := range uf.f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Recv == nil || len(fd.Recv.List) == 0 {
				continue
			}
			s := u.suiteOf(fd.Recv.List[0].Type)
			fn, _ := u.info.Defs[fd.Name].(*types.Func)
			if s == nil || !s.migrated() || fn == nil {
				continue
			}
			name := fd.Name.Name
			if newName, ok := lifecycleRenames[name]; ok {
				u.hooks[methodKey(fn)] = newName
			} else if strings.HasPrefix(name, "Test") {
				u.hooks[methodKey(fn)] = name
			}
		}
	}
}
