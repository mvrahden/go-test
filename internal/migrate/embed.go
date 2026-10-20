package migrate

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestast"
	"golang.org/x/tools/go/ast/astutil"
)

// embedding is a field through which a package-level struct embeds a
// testify suite the unit declares, its base: the migration turns it into a
// named field.
type embedding struct {
	field *ast.Field
	base  *suiteDecl
	orig  string // the field's name while embedded: the base type's name
	index int    // the field's index in the struct
	name  string // the named field it becomes
	ptr   bool   // embedded by pointer; the named field holds the base by value
	owner *suiteDecl
}

// forward is a method a child declares to call the one its base gave it by
// promotion: gotest calls only methods a suite declares itself.
type forward struct {
	name  string // the gotest name, on the child and on the base
	field string
}

// OutsideBaseError is a suite built on a testify suite declared outside the
// test files being migrated: migrate converts a base only together with the
// suites that embed it, so nothing is migrated.
type OutsideBaseError struct {
	File string
	Line int
	Msg  string
}

func (e *OutsideBaseError) Error() string {
	return e.File + ":" + strconv.Itoa(e.Line) + ": " + e.Msg
}

// embedAt is the embedding of field index i, nil for any other field.
func (s *suiteDecl) embedAt(i int) *embedding {
	for _, e := range s.embeds {
		if e.index == i {
			return e
		}
	}
	return nil
}

// findChildren makes a suite of each package-level struct that embeds a
// testify suite not directly but through a base: candidates are the type
// specs findSuites saw embed one. A base the unit declares is converted
// with its child; one declared elsewhere fails the run.
func (u *unit) findChildren(cands []childCand, refused map[*unitFile]bool) error {
	var children []*suiteDecl
	for _, c := range cands {
		if c.ts.TypeParams != nil {
			c.uf.marks = append(c.uf.marks, mark{pos: specPos(c.gd, c.ts), msg: c.obj.Name() + " is a generic testify suite; gotest instantiates generic suites through type aliases — migrate it by hand"})
			refused[c.uf] = true
			continue
		}
		s := &suiteDecl{obj: c.obj, spec: c.ts, file: c.uf, newName: DeriveNewName(c.obj.Name()), st: c.st}
		u.suites = append(u.suites, s)
		u.byKey[objKey(c.obj)] = s
		children = append(children, s)
	}
	for _, s := range children {
		index := 0
		for _, field := range s.st.Fields.List {
			n := max(len(field.Names), 1)
			if len(field.Names) == 0 {
				if err := u.embed(s, field, index, refused); err != nil {
					return err
				}
			}
			index += n
		}
		u.nameEmbeds(s)
	}
	return nil
}

// childCand is a package-level struct type embedding a testify suite
// through another type.
type childCand struct {
	uf  *unitFile
	gd  *ast.GenDecl
	ts  *ast.TypeSpec
	st  *ast.StructType
	obj *types.TypeName
}

// embed records field of child s when it embeds a testify suite.
func (u *unit) embed(s *suiteDecl, field *ast.Field, index int, refused map[*unitFile]bool) error {
	t := u.info.TypeOf(field.Type)
	if t == nil || !embedsTestifySuite(t, map[types.Type]bool{}) {
		return nil
	}
	_, ptr := t.(*types.Pointer)
	if ptr {
		t = t.(*types.Pointer).Elem()
	}
	named, ok := types.Unalias(t).(*types.Named)
	if !ok {
		return nil
	}
	if base := u.suiteFor(named.Obj()); base != nil {
		s.embeds = append(s.embeds, &embedding{field: field, base: base, orig: named.Obj().Name(), index: index, ptr: ptr, owner: s})
		u.embedField[field] = s.embeds[len(s.embeds)-1]
		return nil
	}
	if u.fileAt(named.Obj().Pos()) != nil {
		// A testify suite of these files that migrate leaves as it is.
		s.refusal = s.obj.Name() + " embeds " + named.Obj().Name() + ", which is left for hand migration — migrate this file by hand"
		s.file.marks = append(s.file.marks, mark{pos: s.spec.Pos(), msg: s.refusal})
		refused[s.file] = true
		return nil
	}
	pkg := u.filePkg[s.file.f]
	qual := func(p *types.Package) string {
		if p == pkg {
			return ""
		}
		return p.Name()
	}
	if bp := named.Obj().Pkg().Path(); pkg != nil && (bp == pkg.Path() || bp+"_test" == pkg.Path()) {
		// A non-test file of this directory, which migrate leaves as it is.
		s.refusal = s.obj.Name() + " embeds " + types.TypeString(named, qual) + ", declared in " + u.declFile(named.Obj(), filepath.Dir(s.file.path)) + ", which is not a test file; migrate changes only test files — move " + named.Obj().Name() + " into a _test.go file, or migrate this file by hand"
		s.file.marks = append(s.file.marks, mark{pos: s.spec.Pos(), msg: s.refusal})
		refused[s.file] = true
		return nil
	}
	pos := u.fset.Position(field.Pos())
	return &OutsideBaseError{
		File: pos.Filename,
		Line: pos.Line,
		Msg: s.obj.Name() + " embeds " + types.TypeString(named, qual) + ", a testify suite declared outside the test files being migrated, in package " + named.Obj().Pkg().Path() +
			"; migrate converts a base suite together with the suites that embed it — migrate " + named.Obj().Pkg().Path() + " by hand, or move the base into these test files",
	}
}

// declFile is the base name of the non-test file in dir declaring the
// type obj: an external test package sees it through export data.
func (u *unit) declFile(obj types.Object, dir string) string {
	if name := u.fset.Position(obj.Pos()).Filename; name != "" {
		return filepath.Base(name)
	}
	paths, _ := filepath.Glob(filepath.Join(dir, "*.go"))
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		if err != nil {
			continue
		}
		for _, decl := range f.Decls {
			if gd, ok := decl.(*ast.GenDecl); ok && gd.Tok == token.TYPE {
				for _, spec := range gd.Specs {
					if ts := spec.(*ast.TypeSpec); ts.Name.Name == obj.Name() {
						return filepath.Base(path)
					}
				}
			}
		}
	}
	return "a file of package " + obj.Pkg().Path()
}

// nameEmbeds names the fields s's embeddings become: base, base2, ... the
// first name nothing of s already answers to. A name promoted through one
// of the embeddings stays reachable, through the new field.
func (u *unit) nameEmbeds(s *suiteDecl) {
	taken := map[string]bool{}
	for _, e := range s.embeds {
		for i := 1; ; i++ {
			name := "base"
			if i > 1 {
				name += strconv.Itoa(i)
			}
			if taken[name] {
				continue
			}
			obj, index, _ := types.LookupFieldOrMethod(s.obj.Type(), true, s.obj.Pkg(), name)
			if (obj == nil && index == nil) || (obj != nil && len(index) > 1 && s.embedAt(index[0]) != nil) {
				e.name = name
				taken[name] = true
				break
			}
		}
	}
}

// helperName is the name a base no runner ran keeps: its own, or with
// Helpers appended when gotest would take that name for a suite.
func helperName(name string) string {
	if gotestast.IS_TEST_SUITE.MatchString(name) {
		return name + "Helpers"
	}
	return name
}

// ranSuites are the suites a suite.Run in the unit's files runs.
func (u *unit) ranSuites() map[*suiteDecl]bool {
	ran := map[*suiteDecl]bool{}
	for _, uf := range u.files {
		if uf.skipped() {
			continue
		}
		ast.Inspect(uf.f, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok && u.isSuiteRun(call) {
				if s := u.suiteOfType(u.info.TypeOf(call.Args[1])); s != nil {
					ran[s] = true
				}
			}
			return true
		})
	}
	return ran
}

// markHelpers makes each base no runner ran a helper type: the suites
// embedding it call its methods, gotest never runs it.
func (u *unit) markHelpers() {
	ran := u.ranSuites()
	for _, s := range u.suites {
		for _, e := range s.embeds {
			if b := e.base; !ran[b] && !b.helper {
				b.helper = true
				b.newName = helperName(b.obj.Name())
			}
		}
	}
}

// collectNeedsT gives t to each method of a base that uses what suite.Suite
// provided through its receiver, or calls a method that gains t: once the
// base is a helper, t is how the method reaches the test. A method of a
// child gains t when it calls one that does.
func (u *unit) collectNeedsT() {
	u.needsT = map[string]bool{}
	bases, children := map[*suiteDecl]bool{}, map[*suiteDecl]bool{}
	for _, s := range u.suites {
		for _, e := range s.embeds {
			if e.base.migrated() {
				bases[e.base] = true
			}
			if s.migrated() {
				children[s] = true
			}
		}
	}
	for changed := true; changed; {
		changed = false
		for _, uf := range u.files {
			if uf.refused || uf.skipped() {
				continue
			}
			for _, decl := range uf.f.Decls {
				fd, ok := decl.(*ast.FuncDecl)
				if !ok || fd.Recv == nil || len(fd.Recv.List) == 0 || fd.Body == nil {
					continue
				}
				fn, _ := u.info.Defs[fd.Name].(*types.Func)
				owner := u.suiteOf(fd.Recv.List[0].Type)
				if fn == nil || (!bases[owner] && !children[owner]) {
					continue
				}
				key := methodKey(fn)
				if _, ok := u.hooks[key]; ok {
					continue
				}
				if _, kind, _ := tParam(fd, u.info); kind != "" {
					continue // it has a t of its own
				}
				var recv types.Object
				if names := fd.Recv.List[0].Names; len(names) > 0 && bases[owner] {
					recv = u.info.Defs[names[0]]
				}
				if u.reachesTest(fd.Body, recv) {
					u.hooks[key] = fd.Name.Name
					u.needsT[key] = true
					changed = true
				}
			}
		}
	}
}

// reachesTest reports whether body uses testify through recv, or calls a
// method that takes t.
func (u *unit) reachesTest(body *ast.BlockStmt, recv types.Object) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		x, ok := n.(*ast.SelectorExpr)
		if !ok || found {
			return !found
		}
		sel := u.info.Selections[x]
		if sel == nil {
			return true
		}
		if ident, ok := x.X.(*ast.Ident); ok && recv != nil && u.info.Uses[ident] == recv && testifyObj(sel.Obj()) {
			found = true
		}
		if fn, ok := sel.Obj().(*types.Func); ok && sel.Kind() == types.MethodVal {
			if _, ok := u.hooks[methodKey(fn)]; ok {
				found = true
			}
		}
		return !found
	})
	return found
}

// collectForwards lists, for each migrated suite holding a base, the hooks
// and tests testify ran on it by promotion and it does not declare itself.
func (u *unit) collectForwards() {
	order := map[string]int{"BeforeAll": 0, "AfterAll": 1, "BeforeEach": 2, "AfterEach": 3}
	for _, s := range u.suites {
		if !s.migrated() || len(s.embeds) == 0 {
			continue
		}
		mset := types.NewMethodSet(types.NewPointer(s.obj.Type()))
		for i := range mset.Len() {
			sel := mset.At(i)
			idx := sel.Index()
			if len(idx) < 2 {
				continue
			}
			e := s.embedAt(idx[0])
			if e == nil {
				continue
			}
			name := sel.Obj().Name()
			target := lifecycleRenames[name]
			if target == "" {
				if !strings.HasPrefix(name, "Test") || name == "Test" {
					continue
				}
				target = name
			}
			if own := mset.Lookup(s.obj.Pkg(), target); own != nil && len(own.Index()) == 1 {
				continue // the child's own
			}
			s.forwards = append(s.forwards, forward{name: target, field: e.name})
		}
		sort.SliceStable(s.forwards, func(i, j int) bool {
			oi, iHook := order[s.forwards[i].name]
			oj, jHook := order[s.forwards[j].name]
			switch {
			case iHook && jHook:
				return oi < oj
			case iHook != jHook:
				return iHook
			}
			return s.forwards[i].name < s.forwards[j].name
		})
	}
}

// rewriteEmbedded spells out, in f, every selection that went through an
// embedding the migration names, and renames the keys of composite
// literals that set one.
func (u *unit) rewriteEmbedded(f *ast.File) bool {
	changed := false
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.SelectorExpr:
			changed = u.rewriteSelector(x) || changed
		case *ast.CompositeLit:
			s := u.suiteOfType(u.info.TypeOf(x))
			if s == nil || !s.migrated() {
				return true
			}
			for _, elt := range x.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				key, ok := kv.Key.(*ast.Ident)
				if !ok {
					continue
				}
				for _, e := range s.embeds {
					if key.Name == e.orig {
						key.Name = e.name
						changed = true
					}
				}
			}
		}
		return true
	})
	return changed
}

// rewriteSelector follows x's index path: each embedding on the way gets
// its field name spelled, and a selection of the embedded field itself
// takes the new name. What suite.Suite provided is left to the assertion
// rewrites and to markers.
func (u *unit) rewriteSelector(x *ast.SelectorExpr) bool {
	sel := u.info.Selections[x]
	if sel == nil || testifyObj(sel.Obj()) || isSuiteField(sel.Obj()) {
		return false
	}
	path := sel.Index()
	t := sel.Recv()
	var chain []string
	changed := false
	for i, idx := range path {
		var e *embedding
		if s := u.suiteOfType(t); s != nil && s.migrated() {
			e = s.embedAt(idx)
		}
		if i == len(path)-1 {
			if e != nil && sel.Kind() == types.FieldVal && x.Sel.Name != e.name {
				x.Sel.Name = e.name
				changed = true
			}
			break
		}
		if e != nil {
			chain = append(chain, e.name)
		}
		if p, ok := t.(*types.Pointer); ok {
			t = p.Elem()
		}
		st, ok := t.Underlying().(*types.Struct)
		if !ok {
			break
		}
		t = st.Field(idx).Type()
	}
	for _, name := range chain {
		x.X = &ast.SelectorExpr{X: x.X, Sel: &ast.Ident{Name: name, NamePos: x.Sel.Pos()}}
		changed = true
	}
	return changed
}

// isSuiteField reports whether obj is an embedded suite.Suite field.
func isSuiteField(obj types.Object) bool {
	v, ok := obj.(*types.Var)
	if !ok || !v.Embedded() {
		return false
	}
	t := v.Type()
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	n, ok := t.(*types.Named)
	return ok && n.Obj().Pkg() != nil && n.Obj().Pkg().Path() == suitePkg
}

// forwardsIn renders the forwarding methods of the suites uf declares, by
// the suite's new name, in the order collectForwards gave them.
func (u *unit) forwardsIn(uf *unitFile) map[string][]renderedForward {
	out := map[string][]renderedForward{}
	for _, s := range u.suites {
		if s.file != uf || !s.migrated() || len(s.forwards) == 0 {
			continue
		}
		recv, t := u.recvName(s), "t"
		if recv == t {
			t = "gt"
		}
		for _, fw := range s.forwards {
			out[s.newName] = append(out[s.newName], renderedForward{
				name: fw.name,
				test: strings.HasPrefix(fw.name, "Test"),
				text: fmt.Sprintf("func (%[1]s *%[2]s) %[3]s(%[5]s *gotest.T) {\n\t%[1]s.%[4]s.%[3]s(%[5]s)\n}", recv, s.newName, fw.name, fw.field, t),
			})
		}
	}
	return out
}

// renderedForward is one forwarding method's source.
type renderedForward struct {
	name string
	test bool
	text string
}

// recvName is the receiver name s's methods use, s when none names one.
func (u *unit) recvName(s *suiteDecl) string {
	for _, uf := range u.files {
		for _, decl := range uf.f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Recv == nil || len(fd.Recv.List) == 0 || len(fd.Recv.List[0].Names) == 0 {
				continue
			}
			if name := fd.Recv.List[0].Names[0].Name; name != "_" && u.suiteOf(fd.Recv.List[0].Type) == s {
				return name
			}
		}
	}
	return "s"
}

// insertForwards places each type's forwarding hooks after the declaration
// of the type in src, and each forwarded test before the first test the
// type declares in src whose name sorts after it, or after its last: gotest
// runs tests in source order, testify ran them in name order.
func insertForwards(src []byte, forwards map[string][]renderedForward) ([]byte, error) {
	if len(forwards) == 0 {
		return src, nil
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	lineEnd := func(pos token.Pos) int {
		at := fset.Position(pos).Offset
		if nl := bytes.IndexByte(src[at:], '\n'); nl >= 0 {
			return at + nl
		}
		return len(src)
	}
	type insert struct {
		at, seq int
		text    string
	}
	var inserts []insert
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok || len(forwards[ts.Name.Name]) == 0 {
				continue
			}
			tests := declaredTests(f, ts.Name.Name)
			after := lineEnd(gd.End())
			if len(tests) > 0 {
				after = lineEnd(tests[len(tests)-1].End())
			}
			for _, fw := range forwards[ts.Name.Name] {
				in := insert{at: lineEnd(gd.End()), seq: len(inserts), text: "\n\n" + fw.text}
				if fw.test {
					in.at = after
					for _, fd := range tests {
						if fd.Name.Name > fw.name {
							start := fd.Pos()
							if fd.Doc != nil {
								start = fd.Doc.Pos()
							}
							in.at, in.text = fset.Position(start).Offset, fw.text+"\n\n"
							break
						}
					}
				}
				inserts = append(inserts, in)
			}
		}
	}
	// Bottom-up, so earlier offsets stay valid; at one offset the earlier
	// forward goes in last, which leaves it first.
	sort.Slice(inserts, func(i, j int) bool {
		if inserts[i].at != inserts[j].at {
			return inserts[i].at > inserts[j].at
		}
		return inserts[i].seq > inserts[j].seq
	})
	out := bytes.Clone(src)
	for _, in := range inserts {
		out = append(out[:in.at], append([]byte(in.text), out[in.at:]...)...)
	}
	return out, nil
}

// declaredTests lists the Test methods f declares on the type named name,
// in source order.
func declaredTests(f *ast.File, name string) []*ast.FuncDecl {
	var tests []*ast.FuncDecl
	for _, decl := range f.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Recv == nil || len(fd.Recv.List) == 0 || !strings.HasPrefix(fd.Name.Name, "Test") {
			continue
		}
		typ := fd.Recv.List[0].Type
		if star, ok := typ.(*ast.StarExpr); ok {
			typ = star.X
		}
		if id, ok := typ.(*ast.Ident); ok && id.Name == name {
			tests = append(tests, fd)
		}
	}
	return tests
}

// hasImport reports whether f's import declarations hold path.
func hasImport(f *ast.File, path string) bool {
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.IMPORT {
			continue
		}
		for _, spec := range gd.Specs {
			if is, ok := spec.(*ast.ImportSpec); ok && strings.Trim(is.Path.Value, `"`) == path {
				return true
			}
		}
	}
	return false
}

// ptrEmbedding is the embedding by pointer x selects as a field, nil when
// x selects anything else.
func (u *unit) ptrEmbedding(x *ast.SelectorExpr) *embedding {
	sel := u.info.Selections[x]
	if sel == nil || sel.Kind() != types.FieldVal {
		return nil
	}
	v, ok := sel.Obj().(*types.Var)
	if !ok || !v.Embedded() {
		return nil
	}
	s, path := u.suiteOfType(sel.Recv()), sel.Index()
	for _, idx := range path[:len(path)-1] {
		if s == nil || s.embedAt(idx) == nil {
			return nil
		}
		s = s.embedAt(idx).base
	}
	if s == nil {
		return nil
	}
	if e := s.embedAt(path[len(path)-1]); e != nil && e.ptr {
		return e
	}
	return nil
}

// freshBase reports whether x is &B{...} or new(B) for the base e embeds:
// a pointer nothing else holds, which the base held by value replaces.
func (u *unit) freshBase(x ast.Expr, e *embedding) bool {
	switch v := ast.Unparen(x).(type) {
	case *ast.UnaryExpr:
		lit, ok := v.X.(*ast.CompositeLit)
		return ok && v.Op == token.AND && u.suiteOf(lit.Type) == e.base
	case *ast.CallExpr:
		ident, ok := v.Fun.(*ast.Ident)
		if !ok || len(v.Args) != 1 {
			return false
		}
		b, ok := u.info.Uses[ident].(*types.Builtin)
		return ok && b.Name() == "new" && u.suiteOf(v.Args[0]) == e.base
	}
	return false
}

// ptrFieldSet reports whether the unit's code assigns the pointer e
// embeds, takes its address, or sets it in a literal to anything but a
// fresh base: the base held by value cannot follow.
func (u *unit) ptrFieldSet(e *embedding) bool {
	found := false
	for _, uf := range u.files {
		if uf.skipped() || found {
			continue
		}
		ast.PreorderStack(uf.f, nil, func(n ast.Node, stack []ast.Node) bool {
			switch x := n.(type) {
			case *ast.SelectorExpr:
				if u.ptrEmbedding(x) != e || len(stack) == 0 {
					return !found
				}
				switch p := stack[len(stack)-1].(type) {
				case *ast.AssignStmt:
					found = found || slices.Contains(p.Lhs, ast.Expr(x))
				case *ast.UnaryExpr:
					found = found || p.Op == token.AND
				case *ast.RangeStmt:
					found = found || p.Key == ast.Expr(x) || p.Value == ast.Expr(x)
				}
			case *ast.CompositeLit:
				if u.suiteOfType(u.info.TypeOf(x)) != e.owner {
					return !found
				}
				for i, elt := range x.Elts {
					if kv, ok := elt.(*ast.KeyValueExpr); ok {
						if key, ok := kv.Key.(*ast.Ident); ok && key.Name == e.orig && !u.freshBase(kv.Value, e) {
							found = true
						}
					} else if i == e.index && !u.freshBase(elt, e) {
						found = true
					}
				}
			}
			return !found
		})
	}
	return found
}

// ptrRunsFresh reports whether a suite.Run runs the suite holding e, and
// every one gives it a literal setting e's pointer to a fresh base: run
// with that pointer nil, testify panicked. A run of a suite holding that
// one through a base fails it.
func (u *unit) ptrRunsFresh(e *embedding) bool {
	runs, fresh := 0, true
	for _, uf := range u.files {
		if uf.skipped() {
			continue
		}
		ast.Inspect(uf.f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || !u.isSuiteRun(call) {
				return true
			}
			if s := u.suiteOfType(u.info.TypeOf(call.Args[1])); s != e.owner {
				// Its literal sets the owner's pointer out of sight.
				fresh = fresh && (s == nil || !holds(s, e.owner, map[*suiteDecl]bool{}))
				return true
			}
			runs++
			set := false
			if a, ok := ast.Unparen(call.Args[1]).(*ast.UnaryExpr); ok && a.Op == token.AND {
				if lit, ok := a.X.(*ast.CompositeLit); ok {
					for _, elt := range lit.Elts {
						if kv, ok := elt.(*ast.KeyValueExpr); ok {
							key, ok := kv.Key.(*ast.Ident)
							set = set || ok && key.Name == e.orig && u.freshBase(kv.Value, e)
						}
					}
				}
			}
			fresh = fresh && set
			return true
		})
	}
	return runs > 0 && fresh
}

// holds reports whether s embeds base through its bases, at any depth.
func holds(s, base *suiteDecl, seen map[*suiteDecl]bool) bool {
	if seen[s] {
		return false
	}
	seen[s] = true
	for _, e := range s.embeds {
		if e.base == base || holds(e.base, base, seen) {
			return true
		}
	}
	return false
}

// derefPtrEmbeds rewrites, in f, what the base held by value changes for
// an embedding by pointer: the field selected as a value takes &, *field
// is the field, and a literal's fresh base becomes a value.
func (u *unit) derefPtrEmbeds(f *ast.File) bool {
	changed := false
	astutil.Apply(f, nil, func(c *astutil.Cursor) bool {
		switch x := c.Node().(type) {
		case *ast.SelectorExpr:
			e := u.ptrEmbedding(x)
			if e == nil || !e.owner.migrated() {
				return true
			}
			switch p := c.Parent().(type) {
			case *ast.SelectorExpr:
				if p.X == ast.Expr(x) {
					return true
				}
			case *ast.StarExpr:
				return true // the StarExpr case drops the *
			}
			c.Replace(&ast.UnaryExpr{OpPos: x.Pos(), Op: token.AND, X: x})
			changed = true
		case *ast.StarExpr:
			if sx, ok := x.X.(*ast.SelectorExpr); ok {
				if e := u.ptrEmbedding(sx); e != nil && e.owner.migrated() {
					c.Replace(sx)
					changed = true
				}
			}
		case *ast.CompositeLit:
			s := u.suiteOfType(u.info.TypeOf(x))
			if s == nil || !s.migrated() {
				return true
			}
			for _, e := range s.embeds {
				if !e.ptr {
					continue
				}
				for i, elt := range x.Elts {
					if kv, ok := elt.(*ast.KeyValueExpr); ok {
						if key, ok := kv.Key.(*ast.Ident); ok && key.Name == e.orig {
							kv.Value = valueBase(kv.Value)
							changed = true
						}
					} else if i == e.index {
						x.Elts[i] = valueBase(elt)
						changed = true
					}
				}
			}
		}
		return true
	})
	return changed
}

// valueBase is the value a fresh base &B{...} or new(B) points at.
func valueBase(x ast.Expr) ast.Expr {
	switch v := ast.Unparen(x).(type) {
	case *ast.UnaryExpr:
		return v.X
	case *ast.CallExpr:
		return &ast.CompositeLit{Type: v.Args[0], Lbrace: v.Lparen, Rbrace: v.Rparen}
	}
	return x
}
