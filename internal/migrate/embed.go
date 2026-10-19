package migrate

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"sort"
	"strconv"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestast"
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
	ptr   bool
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
		s.embeds = append(s.embeds, &embedding{field: field, base: base, orig: named.Obj().Name(), index: index, ptr: ptr})
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
	pos := u.fset.Position(field.Pos())
	return &OutsideBaseError{
		File: pos.Filename,
		Line: pos.Line,
		Msg: s.obj.Name() + " embeds " + types.TypeString(named, qual) + ", a testify suite declared outside the test files being migrated, in package " + named.Obj().Pkg().Path() +
			"; migrate converts a base suite together with the suites that embed it — migrate " + named.Obj().Pkg().Path() + " by hand, or move the base into these test files",
	}
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
// base is a helper, t is how the method reaches the test.
func (u *unit) collectNeedsT() {
	u.needsT = map[string]bool{}
	bases := map[*suiteDecl]bool{}
	for _, s := range u.suites {
		for _, e := range s.embeds {
			if e.base.migrated() {
				bases[e.base] = true
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
				if fn == nil || !bases[u.suiteOf(fd.Recv.List[0].Type)] {
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
				if names := fd.Recv.List[0].Names; len(names) > 0 {
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
// the suite's new name.
func (u *unit) forwardsIn(uf *unitFile) map[string]string {
	out := map[string]string{}
	for _, s := range u.suites {
		if s.file != uf || !s.migrated() || len(s.forwards) == 0 {
			continue
		}
		recv, t := u.recvName(s), "t"
		if recv == t {
			t = "gt"
		}
		var b strings.Builder
		for _, fw := range s.forwards {
			fmt.Fprintf(&b, "\n\nfunc (%[1]s *%[2]s) %[3]s(%[5]s *gotest.T) {\n\t%[1]s.%[4]s.%[3]s(%[5]s)\n}", recv, s.newName, fw.name, fw.field, t)
		}
		out[s.newName] = b.String()
	}
	return out
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

// insertForwards places each type's forwarding methods after the
// declaration of the type in src.
func insertForwards(src []byte, forwards map[string]string) ([]byte, error) {
	if len(forwards) == 0 {
		return src, nil
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	type insert struct {
		at   int
		text string
	}
	var inserts []insert
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok || forwards[ts.Name.Name] == "" {
				continue
			}
			at := fset.Position(gd.End()).Offset
			if nl := bytes.IndexByte(src[at:], '\n'); nl >= 0 {
				at += nl
			} else {
				at = len(src)
			}
			inserts = append(inserts, insert{at: at, text: forwards[ts.Name.Name]})
		}
	}
	sort.Slice(inserts, func(i, j int) bool { return inserts[i].at > inserts[j].at })
	out := bytes.Clone(src)
	for _, in := range inserts {
		out = append(out[:in.at], append([]byte(in.text), out[in.at:]...)...)
	}
	return out, nil
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
