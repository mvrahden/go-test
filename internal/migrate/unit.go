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

	"golang.org/x/tools/go/types/typeutil"

	"github.com/mvrahden/go-test/internal/gotestast"
	"github.com/mvrahden/go-test/internal/protocol"
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
	// generated: the file is headed "Code generated ... DO NOT EDIT." and
	// is left as it is; its generator owns it.
	generated bool
}

// skipped reports whether the migration leaves uf as it is: generated, or
// out of every configuration it type-checks.
func (uf *unitFile) skipped() bool { return uf.generated || uf.unchecked != "" }

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
	// needsT marks, by methodKey, the methods of a base that gain t
	// although gotest does not call them.
	needsT map[string]bool
	// embedField maps the fields through which a child embeds its base.
	embedField map[*ast.Field]*embedding
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
	// reserved maps the suite's methods named as gotest reserves, which
	// testify did not run, to their markers.
	reserved map[string]string
	// testifyHooks names the testify hooks the suite declares.
	testifyHooks map[string]bool
	// setupStops: the SetupSuite testify ran on it, its own or a base's,
	// can stop early, on an assertion, a fatal call or a panic.
	setupStops bool
	// st is the struct of a suite that embeds its base; embeds are the
	// fields it does so through.
	st     *ast.StructType
	embeds []*embedding
	// generic: the suite has type parameters; gd declares it.
	generic bool
	gd      *ast.GenDecl
	// helper: a base no suite.Run ran; it is migrated as a helper type
	// the suites embedding it call, which gotest does not run.
	helper   bool
	forwards []forward
}

func (s *suiteDecl) migrated() bool { return s.refusal == "" }

// declaresTest reports whether testify runs a test on the suite obj names:
// a Test method of its own or one a base promotes.
func declaresTest(obj *types.TypeName) bool {
	mset := types.NewMethodSet(types.NewPointer(obj.Type()))
	for i := range mset.Len() {
		if name := mset.At(i).Obj().Name(); strings.HasPrefix(name, "Test") && name != "Test" {
			return true
		}
	}
	return false
}

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
		case *ast.IndexExpr: // an instance of a generic type
			expr = x.X
			continue
		case *ast.IndexListExpr:
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
// suite.Suite embedded, directly or through a base. A suite whose base
// is declared outside these test files is an error.
func (u *unit) findSuites() (map[*unitFile]bool, error) {
	u.byKey = map[string]*suiteDecl{}
	u.embedField = map[*ast.Field]*embedding{}
	refused := map[*unitFile]bool{}
	var cands []childCand
	for _, uf := range u.files {
		if uf.skipped() {
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
				case direct:
					s := &suiteDecl{obj: obj, spec: ts, file: uf, newName: DeriveNewName(obj.Name()), generic: ts.TypeParams != nil, gd: gd}
					u.suites = append(u.suites, s)
					u.byKey[objKey(obj)] = s
				default:
					if embedsTestifySuite(obj.Type(), map[types.Type]bool{}) {
						cands = append(cands, childCand{uf: uf, gd: gd, ts: ts, st: st, obj: obj})
					}
				}
			}
		}
	}
	return refused, u.findChildren(cands, refused)
}

// specPos is where a marker for ts goes: above its own line, or above the
// declaration when it is not grouped.
func specPos(gd *ast.GenDecl, ts *ast.TypeSpec) token.Pos {
	if !gd.Lparen.IsValid() {
		return gd.Pos()
	}
	return ts.Pos()
}

// refuse decides which suites stay as they are. A package-level struct
// embedding a suite migrates with it; by pointer only when every
// suite.Run sets that pointer to a fresh base and nothing else does. Any
// other struct embedding one, local or anonymous, would strand the child,
// so the suite is refused with the file declaring it and the file
// declaring the child.
// So is a suite whose new name is taken, and one a file migrate cannot
// type-check refers to. A refused file is left whole, so every suite it
// declares or refers to is refused too, and a child goes with its base,
// until nothing changes.
func (u *unit) refuse(refused map[*unitFile]bool) {
	for _, uf := range u.files {
		if uf.skipped() {
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
				e := u.embedField[field]
				if e != nil && (!e.ptr || u.ptrRunsFresh(e) && !u.ptrFieldSet(e)) {
					continue // the migration names the field
				}
				child, pos, local, generic := "", field.Pos(), false, false
				for i := len(stack) - 1; i >= 0; i-- {
					switch x := stack[i].(type) {
					case *ast.FuncDecl, *ast.FuncLit:
						local = true
					case *ast.TypeSpec:
						if child == "" && x.Type == st {
							child, pos, generic = x.Name.Name, x.Pos(), x.TypeParams != nil
							if gd, ok := stack[max(i-1, 0)].(*ast.GenDecl); ok {
								pos = specPos(gd, x)
							}
						}
					}
				}
				var msg string
				switch {
				case e != nil:
					msg = child + " embeds *" + base.obj.Name() + ", which not every suite.Run sets to a fresh &" + base.obj.Name() + "{}, or other code sets; gotest creates a suite with new, which leaves that pointer nil — embed " + base.obj.Name() + " by value, or migrate this file by hand"
				case child == "":
					msg = "an anonymous struct embeds " + base.obj.Name() + "; gotest runs only suites declared at package level — declare it as a package-level type, or migrate this file by hand"
				case local:
					msg = child + " embeds " + base.obj.Name() + " inside a function; gotest runs only suites declared at package level — declare " + child + " at package level, or migrate this file by hand"
				case !generic: // a generic child carries its own marker
					msg = child + " embeds " + base.obj.Name() + ", not suite.Suite — migrate this file by hand"
				}
				if msg != "" {
					uf.marks = append(uf.marks, mark{pos: pos, msg: msg})
				}
				if base.migrated() {
					by := child
					if by == "" {
						by = "an anonymous struct"
					}
					base.refusal = base.obj.Name() + " is embedded by " + by + " in " + filepath.Base(uf.path) + " — migrate this file by hand"
					base.marked = true
				}
				refused[uf] = true
				refused[base.file] = true
			}
			return true
		})
	}

	u.markHelpers()

	// A generic suite migrates only as a base the suites embedding it call.
	for _, s := range u.suites {
		if s.generic && !s.helper && s.migrated() {
			s.refusal = s.obj.Name() + " is a generic testify suite; gotest instantiates generic suites through type aliases — migrate it by hand"
			s.file.marks = append(s.file.marks, mark{pos: specPos(s.gd, s.spec), msg: s.refusal})
			refused[s.file] = true
		}
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
		if !s.migrated() {
			continue
		}
		if !s.helper && !declaresTest(s.obj) {
			s.refusal = s.obj.Name() + " declares no test; gotest refuses a suite without a Test, Benchmark or Fuzz method — add a test, or migrate this file by hand"
			s.file.marks = append(s.file.marks, mark{pos: s.spec.Pos(), msg: s.refusal})
			refused[s.file] = true
			continue
		}
		if !s.helper && token.IsExported(s.newName) && !runsPlain(s.newName) {
			s.refusal = s.obj.Name() + " would become " + s.newName + ", which gotest does not run as a plain suite — rename it and migrate this file by hand"
			s.file.marks = append(s.file.marks, mark{pos: s.spec.Pos(), msg: s.refusal})
			refused[s.file] = true
			continue
		}
		if s.newName == s.obj.Name() {
			continue
		}
		key := s.obj.Pkg().Path() + "." + s.newName
		clash := taken[key]
		for _, scope := range u.scopes[s.obj.Pkg().Path()] {
			clash = clash || scope.Lookup(s.newName) != nil
		}
		// An external test package dot-importing the suite's package
		// shares its names.
		for _, uf := range u.files {
			if pkg := u.filePkg[uf.f]; pkg != nil && pkg.Path() != s.obj.Pkg().Path() && dotImports(uf.f, s.obj.Pkg().Path()) {
				for _, scope := range u.scopes[pkg.Path()] {
					clash = clash || scope.Lookup(s.newName) != nil
				}
			}
		}
		taken[key] = true
		switch {
		case !s.helper && !token.IsExported(s.newName):
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
	// it is with a marker when it does or when it uses testify; a
	// generated one takes none.
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
		if !uf.generated && (mentions || importsTestify(uf.f)) {
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
				switch {
				case !users[s][uf] || !s.migrated():
				case uf.generated:
					s.refusal = s.obj.Name() + " is used in " + filepath.Base(uf.path) + ", which is generated — migrate it by hand, then regenerate that file"
					s.marked = true
				case refused[uf]:
					s.refusal = s.obj.Name() + " is used in " + filepath.Base(uf.path) + ", which is left for hand migration — migrate this file by hand"
					s.marked = true
				}
			}
			for _, e := range s.embeds {
				if s.migrated() && !e.base.migrated() {
					s.refusal = s.obj.Name() + " embeds " + e.base.obj.Name() + ", which is left for hand migration — migrate this file by hand"
					s.marked = true
				}
			}
			if !s.migrated() {
				refused[s.file] = true
				changed = true
			}
		}
		// A child left as it is still embeds its base, so the base stays too.
		for _, s := range u.suites {
			for _, e := range s.embeds {
				if !s.migrated() && e.base.migrated() {
					e.base.refusal = e.base.obj.Name() + " is embedded by " + s.obj.Name() + " in " + filepath.Base(s.file.path) + " — migrate this file by hand"
					e.base.marked = true
					refused[e.base.file] = true
					changed = true
				}
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

// dotImports reports whether f imports path into its own scope.
func dotImports(f *ast.File, path string) bool {
	for _, is := range f.Imports {
		if is.Name != nil && is.Name.Name == "." && strings.Trim(is.Path.Value, `"`) == path {
			return true
		}
	}
	return false
}

// importsSuite reports whether f imports testify's suite package: a
// generated file importing it is its generator's to migrate.
func importsSuite(f *ast.File) bool {
	for _, is := range f.Imports {
		if strings.Trim(is.Path.Value, `"`) == "github.com/stretchr/testify/suite" {
			return true
		}
	}
	return false
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
	refused, err := u.findSuites()
	if err != nil {
		return err
	}
	u.refuse(refused)

	migrated := false
	for _, s := range u.suites {
		migrated = migrated || s.migrated()
	}
	if migrated {
		u.collectHooks()
		u.collectNeedsT()
		u.collectForwards()
		for _, s := range u.suites {
			s.setupStops = s.migrated() && u.setupMayStop(s)
		}
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
		if !migrated || uf.skipped() {
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
		src, err := insertForwards(buf.Bytes(), u.forwardsIn(uf))
		if err != nil {
			return fmt.Errorf("format %s: %w", uf.path, err)
		}
		out, err := formatImports(uf.path, src)
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
		if uf.refused || uf.skipped() {
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
				if s.testifyHooks == nil {
					s.testifyHooks = map[string]bool{}
				}
				s.testifyHooks[name] = true
			} else if strings.HasPrefix(name, "Test") {
				u.hooks[methodKey(fn)] = name
			}
			if msg := reservedName(name); msg != "" {
				if s.reserved == nil {
					s.reserved = map[string]string{}
				}
				s.reserved[name] = msg
			}
		}
	}
	// A hook renamed onto a name the suite already declares shares that
	// name with it: the marker must not tell the hook to go.
	for _, s := range u.suites {
		for hook, name := range lifecycleRenames {
			if s.reserved[name] != "" && s.testifyHooks[hook] {
				s.reserved[name] = "testify's " + hook + " becomes " + name + ", which the suite already declares — rename the other " + name
			}
		}
	}
}

// setupMayStop reports whether the SetupSuite testify ran on s, its own
// or one promoted from a base, can stop early.
func (u *unit) setupMayStop(s *suiteDecl) bool {
	obj, _, _ := types.LookupFieldOrMethod(types.NewPointer(s.obj.Type()), true, s.obj.Pkg(), "SetupSuite")
	fn, ok := obj.(*types.Func)
	return ok && u.mayStop(fn, map[*types.Func]bool{})
}

// mayStop reports whether calling fn can end it early: a testify assertion,
// which the migration makes fatal, a FailNow, Fatal or Skip, a panic, or a
// call that can. A call into code whose body these files do not hold can,
// when it is handed a T or a suite.
func (u *unit) mayStop(fn *types.Func, seen map[*types.Func]bool) bool {
	fd := u.funcDecl(fn)
	if fd == nil || fd.Body == nil {
		return false
	}
	if seen[fn] {
		return false
	}
	seen[fn] = true
	found := false
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || found {
			return !found
		}
		switch fun := call.Fun.(type) {
		case *ast.Ident:
			b, ok := u.info.Uses[fun].(*types.Builtin)
			found = ok && b.Name() == "panic"
		case *ast.SelectorExpr:
			switch fun.Sel.Name {
			case "FailNow", "Fatal", "Fatalf", "Skip", "Skipf", "SkipNow":
				found = true
			}
		}
		found = found || u.assertionName(call) != "" || u.callMayStop(call, seen)
		return !found
	})
	return found
}

// callMayStop reports whether call reaches a function that can stop: one
// these files declare and mayStop finds stopping, or one declared
// elsewhere that is handed a T or a suite, which it may stop through.
func (u *unit) callMayStop(call *ast.CallExpr, seen map[*types.Func]bool) bool {
	fn, ok := typeutil.Callee(u.info, call).(*types.Func)
	if !ok {
		return false
	}
	if u.funcDecl(fn) != nil {
		return u.mayStop(fn, seen)
	}
	if fn.Pkg() != nil && (fn.Pkg().Path() == "testing" || fn.Pkg().Path() == suitePkg || testifyObj(fn)) {
		return false // the stopping ones are matched by name
	}
	args := call.Args
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok && u.info.Selections[sel] != nil {
		args = append([]ast.Expr{sel.X}, args...)
	}
	for _, a := range args {
		if t := u.info.TypeOf(a); t != nil && (u.suiteOfType(t) != nil || reachesT(t)) {
			return true
		}
	}
	return false
}

// reachesT reports whether t is a testing.T, B, F or TB, or a pointer to one.
func reachesT(t types.Type) bool {
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	n, ok := types.Unalias(t).(*types.Named)
	if !ok || n.Obj().Pkg() == nil || n.Obj().Pkg().Path() != "testing" {
		return false
	}
	switch n.Obj().Name() {
	case "T", "B", "F", "TB":
		return true
	}
	return false
}

// funcDecl is the declaration of fn in these files, nil when they hold none.
func (u *unit) funcDecl(fn *types.Func) *ast.FuncDecl {
	uf := u.fileAt(fn.Pos())
	if uf == nil {
		return nil
	}
	for _, decl := range uf.f.Decls {
		if fd, ok := decl.(*ast.FuncDecl); ok && fd.Name.Pos() == fn.Pos() {
			return fd
		}
	}
	return nil
}

// reservedName is the marker for a suite method gotest gives a meaning
// testify did not, empty for any other name. testify ran the methods
// named Test... and its hooks, which the migration converts.
func reservedName(name string) string {
	role := ""
	switch name {
	case "BeforeAll", "AfterAll", "BeforeEach", "AfterEach":
		role = "a lifecycle hook"
	case "SuiteConfig":
		role = "the suite's configuration"
	case "SuiteGuard":
		role = "the suite's guard"
	case "Test":
		return "testify ran Test as a test; gotest runs only methods named Test followed by more, so it no longer runs — rename it, e.g. TestAll"
	}
	base := strings.TrimPrefix(strings.TrimPrefix(name, "F_"), "X_")
	more := func(prefix string) bool { return len(base) > len(prefix) && strings.HasPrefix(base, prefix) }
	switch {
	case role != "":
	case more(protocol.PrefixBenchmark):
		role = "a benchmark"
	case more(protocol.PrefixFuzz):
		role = "a fuzz target"
	case strings.HasPrefix(name, "F_") && more("Test"):
		role = "a focused test"
	case strings.HasPrefix(name, "X_") && more("Test"):
		role = "an excluded test"
	default:
		return ""
	}
	return "gotest takes a suite method named " + name + " for " + role + "; testify did not — rename it"
}

// runsPlain reports whether gotest runs a suite of that name, neither
// focused nor excluded: "TestSuite" alone is no suite, F_ and X_ prefixes
// focus and exclude.
func runsPlain(name string) bool {
	return gotestast.IS_TEST_SUITE.MatchString(name) && !strings.HasPrefix(name, "F_") && !strings.HasPrefix(name, "X_")
}
