package gotestgen

import (
	"fmt"
	"go/types"
	"strings"
	"unicode"

	"golang.org/x/tools/go/packages"
)

// fuzzFanVersion stamps every generated fuzz identifier. The flatten rules
// and the hybrid-leaf wire format are internal and undocumented on purpose,
// but they are versioned so a rule change can never silently reinterpret a
// cached corpus: the identifiers move, the generated file changes, and the
// old positions are read by nothing.
const fuzzFanVersion = "v1"

// fuzzEmitter builds decoder/encoder source for a package's non-native fuzz
// argument types, memoising one read/write helper pair per composite type.
type fuzzEmitter struct {
	genPkg   *types.Package
	qual     types.Qualifier
	pkgPaths map[string]bool
	idents   map[string]string // type string -> helper identifier
	taken    map[string]bool   // helper identifiers already handed out
	helpers  map[string]string // helper identifier -> emitted source (read + write)
	order    []string          // helper identifiers, in emission order
	stack    []string          // type strings currently being emitted (cycle detection)
	path     []string          // field path, for error messages

	// literals/literalOrder mirror helpers/order, but for the composite
	// literal-rendering functions (ƒ_fuzzlit_*). They are keyed on the same
	// identifiers assignName hands out for helpers/idents, so a type's
	// read/write/literal helpers always share one suffix. litStack mirrors
	// stack's cycle-detection discipline for the literal walk.
	literals     map[string]string
	literalOrder []string
	litStack     []string

	// needsStrings/needsStrconv/needsMath track which standard library
	// packages the emitted literal source actually references, so
	// BuildFuzzCodecs can report exactly the imports the renderer needs to
	// add — never more, since an unused import is a compile error in the
	// generated file.
	needsStrings bool
	needsStrconv bool
	needsMath    bool

	// fans/fanOrder hold the fan-in/fan-out function pairs (ƒ_fuzzin_*/
	// ƒ_fuzzout_*) keyed on the same identifiers assignName hands out;
	// fanInfos memoises the leaf shape per unaliased type string, and
	// fanStack detects recursion during the fan walk. hybrids/hybridOrder
	// hold the mini-codec pairs (ƒ_fuzzdec_*/ƒ_fuzzenc_*) of the types that
	// ride as one []byte leaf.
	fans        map[string]string
	fanOrder    []string
	fanInfos    map[string]*fanInfo
	fanStack    []string
	hybrids     map[string]string
	hybridOrder []string
}

func newFuzzEmitter(pkg *packages.Package) *fuzzEmitter {
	e := &fuzzEmitter{
		genPkg:   pkg.Types,
		pkgPaths: map[string]bool{},
		idents:   map[string]string{},
		taken:    map[string]bool{},
		helpers:  map[string]string{},
		literals: map[string]string{},
		fans:     map[string]string{},
		fanInfos: map[string]*fanInfo{},
		hybrids:  map[string]string{},
	}
	e.qual = func(p *types.Package) string {
		if p == nil || p == e.genPkg {
			return ""
		}
		e.pkgPaths[p.Path()] = true
		return p.Name()
	}
	return e
}

// assignName maps a type string to a stable, unique, valid Go identifier,
// e.g. "[]string" -> "slice_string". Collisions get a numeric suffix, so the
// mapping stays injective.
func (e *fuzzEmitter) assignName(typeStr string) string {
	if name, ok := e.idents[typeStr]; ok {
		return name
	}
	base := sanitizeFuzzTypeIdent(typeStr)
	name := base
	for i := 2; e.taken[name]; i++ {
		name = fmt.Sprintf("%s_%d", base, i)
	}
	e.taken[name] = true
	e.idents[typeStr] = name
	return name
}

func sanitizeFuzzTypeIdent(typeStr string) string {
	s := strings.NewReplacer("[]", "slice_", "*", "ptr_", "[", "arr", "]", "_").Replace(typeStr)
	var b strings.Builder
	for _, r := range s {
		if r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	for strings.Contains(out, "__") {
		out = strings.ReplaceAll(out, "__", "_")
	}
	if out == "" {
		return "anon"
	}
	if unicode.IsDigit(rune(out[0])) {
		return "t_" + out
	}
	return out
}

// readCall returns a Go expression that reads one value of t from the reader
// variable ƒr, emitting (and memoising) whatever helper functions it needs.
func (e *fuzzEmitter) readCall(t types.Type) (string, error) {
	u := types.Unalias(t)

	if named, ok := u.(*types.Named); ok {
		switch under := named.Underlying().(type) {
		case *types.Basic:
			m, err := e.basicRead(under)
			if err != nil {
				return "", err
			}
			return e.typeRef(t) + "(" + m + ")", nil
		case *types.Slice:
			if isUnnamedByte(under.Elem()) {
				return e.typeRef(t) + "(ƒr.ByteSlice())", nil
			}
		}
		return e.helperRead(t)
	}

	switch c := u.(type) {
	case *types.Basic:
		return e.basicRead(c)
	case *types.Slice:
		if isUnnamedByte(c.Elem()) {
			return "ƒr.ByteSlice()", nil
		}
		return e.helperRead(t)
	case *types.Struct, *types.Array, *types.Pointer:
		return e.helperRead(t)
	}
	return "", e.reject(t, fuzzRejectReason(u))
}

// writeStmt returns a Go statement that writes the value of expression src
// (of type t) to the writer variable ƒw.
func (e *fuzzEmitter) writeStmt(t types.Type, src string) (string, error) {
	u := types.Unalias(t)

	if named, ok := u.(*types.Named); ok {
		switch under := named.Underlying().(type) {
		case *types.Basic:
			m, err := e.basicWrite(under)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("ƒw.%s(%s(%s))", m, under.Name(), src), nil
		case *types.Slice:
			if isUnnamedByte(under.Elem()) {
				return fmt.Sprintf("ƒw.ByteSlice([]byte(%s))", src), nil
			}
		}
		return e.helperWrite(t, src)
	}

	switch c := u.(type) {
	case *types.Basic:
		m, err := e.basicWrite(c)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("ƒw.%s(%s)", m, src), nil
	case *types.Slice:
		if isUnnamedByte(c.Elem()) {
			return fmt.Sprintf("ƒw.ByteSlice(%s)", src), nil
		}
		return e.helperWrite(t, src)
	case *types.Struct, *types.Array, *types.Pointer:
		return e.helperWrite(t, src)
	}
	return "", e.reject(t, fuzzRejectReason(u))
}

func (e *fuzzEmitter) helperRead(t types.Type) (string, error) {
	name, err := e.helper(t)
	if err != nil {
		return "", err
	}
	return "ƒ_fuzzread_" + fuzzFanVersion + "_" + name + "(ƒr)", nil
}

func (e *fuzzEmitter) helperWrite(t types.Type, src string) (string, error) {
	name, err := e.helper(t)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("ƒ_fuzzwrite_%s_%s(ƒw, %s)", fuzzFanVersion, name, src), nil
}

// helper emits the read/write pair for a composite type exactly once and
// returns its identifier.
func (e *fuzzEmitter) helper(t types.Type) (string, error) {
	// Keyed on the unaliased type: an alias and its target are the same type,
	// so they must share one helper rather than emitting identical bodies
	// under two names.
	t = types.Unalias(t)
	ts := types.TypeString(t, e.qual)
	for _, s := range e.stack {
		if s == ts {
			return "", fmt.Errorf("type %s is recursive — recursive types are not supported; a depth-limited variant would be needed", ts)
		}
	}
	name := e.assignName(ts)
	if _, done := e.helpers[name]; done {
		return name, nil
	}

	e.stack = append(e.stack, ts)
	src, err := e.helperSource(t, ts, name)
	e.stack = e.stack[:len(e.stack)-1]
	if err != nil {
		return "", err
	}

	e.helpers[name] = src
	e.order = append(e.order, name)
	return name, nil
}

func (e *fuzzEmitter) helperSource(t types.Type, typeRef, name string) (string, error) {
	readName := "ƒ_fuzzread_" + fuzzFanVersion + "_" + name
	writeName := "ƒ_fuzzwrite_" + fuzzFanVersion + "_" + name

	var read, write strings.Builder
	fmt.Fprintf(&read, "\nfunc %s(ƒr *gotestfuzz.Reader) %s {\n", readName, typeRef)
	fmt.Fprintf(&write, "\nfunc %s(ƒw *gotestfuzz.Writer, ƒv %s) {\n", writeName, typeRef)

	switch u := types.Unalias(t).Underlying().(type) {
	case *types.Struct:
		read.WriteString("\tvar ƒv " + typeRef + "\n")
		for i := 0; i < u.NumFields(); i++ {
			f := u.Field(i)
			if f.Name() == "_" {
				continue // blank fields are unreachable and always zero
			}
			r, w, err := e.structField(f)
			if err != nil {
				return "", err
			}
			fmt.Fprintf(&read, "\tƒv.%s = %s\n", f.Name(), r)
			fmt.Fprintf(&write, "\t%s\n", w)
		}
		read.WriteString("\treturn ƒv\n")

	case *types.Slice:
		r, err := e.readCall(u.Elem())
		if err != nil {
			return "", err
		}
		w, err := e.writeStmt(u.Elem(), "ƒv[ƒi]")
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&read, "\tƒn := ƒr.Len()\n\tif ƒn == 0 {\n\t\treturn nil\n\t}\n\tƒv := make(%s, ƒn)\n\tfor ƒi := range ƒv {\n\t\tƒv[ƒi] = %s\n\t}\n\treturn ƒv\n", typeRef, r)
		fmt.Fprintf(&write, "\tƒn := ƒw.Len(len(ƒv))\n\tfor ƒi := 0; ƒi < ƒn; ƒi++ {\n\t\t%s\n\t}\n", w)

	case *types.Array:
		r, err := e.readCall(u.Elem())
		if err != nil {
			return "", err
		}
		w, err := e.writeStmt(u.Elem(), "ƒv[ƒi]")
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&read, "\tvar ƒv %s\n\tfor ƒi := range ƒv {\n\t\tƒv[ƒi] = %s\n\t}\n\treturn ƒv\n", typeRef, r)
		fmt.Fprintf(&write, "\tfor ƒi := range ƒv {\n\t\t%s\n\t}\n", w)

	case *types.Pointer:
		r, err := e.readCall(u.Elem())
		if err != nil {
			return "", err
		}
		w, err := e.writeStmt(u.Elem(), "*ƒv")
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&read, "\tif !ƒr.Bool() {\n\t\treturn nil\n\t}\n\tƒx := %s\n\treturn &ƒx\n", r)
		fmt.Fprintf(&write, "\tƒw.Bool(ƒv != nil)\n\tif ƒv == nil {\n\t\treturn\n\t}\n\t%s\n", w)

	default:
		return "", e.reject(t, fuzzRejectReason(u))
	}

	read.WriteString("}\n")
	write.WriteString("}\n")
	return read.String() + write.String(), nil
}

// structField emits the read expression and write statement for one struct
// field, with the field pushed onto the error path for the duration.
func (e *fuzzEmitter) structField(f *types.Var) (read, write string, err error) {
	e.path = append(e.path, f.Name())
	defer func() { e.path = e.path[:len(e.path)-1] }()

	if !f.Exported() {
		return "", "", e.reject(f.Type(), "unexported fields cannot be set — fuzz the constructor's input, or declare a local wrapper struct")
	}
	read, err = e.readCall(f.Type())
	if err != nil {
		return "", "", err
	}
	write, err = e.writeStmt(f.Type(), "ƒv."+f.Name())
	if err != nil {
		return "", "", err
	}
	return read, write, nil
}

func (e *fuzzEmitter) basicRead(b *types.Basic) (string, error) {
	m, ok := fuzzBasicMethod[b.Kind()]
	if !ok {
		return "", e.reject(b, fuzzRejectReason(b))
	}
	return "ƒr." + m + "()", nil
}

func (e *fuzzEmitter) basicWrite(b *types.Basic) (string, error) {
	m, ok := fuzzBasicMethod[b.Kind()]
	if !ok {
		return "", e.reject(b, fuzzRejectReason(b))
	}
	return m, nil
}

// fuzzBasicMethod maps a basic kind to the FuzzReader/FuzzWriter method pair
// that encodes it. byte and rune are Uint8 and Int32 — the same kinds under
// different names — so they need no separate entries.
var fuzzBasicMethod = map[types.BasicKind]string{
	types.Bool:    "Bool",
	types.Int:     "Int",
	types.Int8:    "Int8",
	types.Int16:   "Int16",
	types.Int32:   "Int32",
	types.Int64:   "Int64",
	types.Uint:    "Uint",
	types.Uint8:   "Uint8",
	types.Uint16:  "Uint16",
	types.Uint32:  "Uint32",
	types.Uint64:  "Uint64",
	types.Float32: "Float32",
	types.Float64: "Float64",
	types.String:  "String",
}

// fuzzRejectReason explains, in the imperative the design doc's exclusion
// table uses, why a type has no honest encoding — and what to do instead.
// Permitting any of these would generate code that lies.
func fuzzRejectReason(t types.Type) string {
	switch u := t.(type) {
	case *types.Map:
		return "maps have no canonical encoding — fuzz a slice of key/value pairs and build the map in the callback"
	case *types.Interface:
		return "no value can be synthesized for an interface"
	case *types.Chan:
		return "no value can be synthesized for a channel"
	case *types.Signature:
		return "no value can be synthesized for a func"
	case *types.TypeParam:
		return "generic fuzz argument types are not supported — instantiate the suite with a concrete type"
	case *types.Basic:
		switch u.Kind() {
		case types.Complex64, types.Complex128:
			return "complex numbers are not fuzzable — fuzz two floats and combine them in the callback"
		case types.Uintptr, types.UnsafePointer:
			return "pointer-sized integers and unsafe pointers are not fuzzable"
		}
	}
	return "not fuzzable"
}

// reject builds the generation-time error, naming the exact field path and
// the offending type — a rejection is a conversation, a false-positive
// crasher is a lost afternoon.
func (e *fuzzEmitter) reject(t types.Type, reason string) error {
	return fmt.Errorf("%s (%s) is not fuzzable — %s",
		strings.Join(e.path, "."), types.TypeString(t, e.qual), reason)
}

func (e *fuzzEmitter) typeRef(t types.Type) string { return types.TypeString(t, e.qual) }

// isLiteralStructShape reports whether t's underlying type is a struct —
// the shape that gets the "&T{...}" pointer literal form rather than the
// "&[]T{elem}[0]" slice-index form.
func isLiteralStructShape(t types.Type) bool {
	u := types.Unalias(t)
	if named, ok := u.(*types.Named); ok {
		u = named.Underlying()
	}
	_, ok := u.(*types.Struct)
	return ok
}

// isUnnamedByte reports whether t is exactly the predeclared byte/uint8 —
// the element type that makes a slice encodable as a length-prefixed blob. A
// named byte type is not, since ƒr.ByteSlice() would yield the wrong slice
// type.
func isUnnamedByte(t types.Type) bool {
	b, ok := types.Unalias(t).(*types.Basic)
	return ok && b.Kind() == types.Uint8
}

// literalFloatHelperTpl is the one shared, non-memoised helper every
// generated file with a reachable float gets at most once. It exists
// because rendering a float needs a conditional (is it NaN? +Inf? -Inf?),
// and a Go expression cannot branch — every literal function that reaches a
// float calls this rather than repeating the branch inline. It is purely an
// implementation detail of the GENERATED FILE: the ƒ_* name never appears in
// the STRING a literal function returns, which is the only thing that gets
// spliced into a user's file, so this helper existing at all does not
// violate the self-contained-string constraint.
const literalFloatHelperTpl = `
func ƒ_fuzzlitfloat_%[1]s(v float64) string {
	if math.IsNaN(v) {
		return "math.NaN()"
	}
	if math.IsInf(v, 1) {
		return "math.Inf(1)"
	}
	if math.IsInf(v, -1) {
		return "math.Inf(-1)"
	}
	return strconv.FormatFloat(v, 'g', -1, 64)
}
`

// literalSupported reports whether t has a self-contained Go literal form —
// the same shapes helperSource emits, minus anything containing an
// unsupported shape. Every pointer whose element is itself supported
// qualifies: a pointer-to-struct renders as "&T{...}", and everything else
// (pointer-to-basic, -slice, -array, -pointer, ...) renders as
// "&[]T{elem}[0]" — slice indexing is an addressable operand per the spec,
// so this is valid Go on every version gotest supports, unlike the bare
// "&5" form. It is a pure read of the type graph: it never emits anything
// and never touches the helpers/literals memoisation maps, so it is safe to
// call speculatively before deciding whether to bother building a literal
// helper at all.
func (e *fuzzEmitter) literalSupported(t types.Type) bool {
	u := types.Unalias(t)
	ts := types.TypeString(u, e.qual)
	for _, s := range e.litStack {
		if s == ts {
			// A cycle here would mean t is recursive, which the read/write
			// pass already rejects with a hard error before literal support
			// is ever consulted for it. This branch is a defensive backstop
			// against an unbounded walk, not a reachable outcome today.
			return false
		}
	}

	underlying := u
	if named, ok := u.(*types.Named); ok {
		underlying = named.Underlying()
	}

	switch under := underlying.(type) {
	case *types.Basic:
		_, ok := fuzzBasicMethod[under.Kind()]
		return ok

	case *types.Slice:
		if isUnnamedByte(under.Elem()) {
			return true
		}
		e.litStack = append(e.litStack, ts)
		ok := e.literalSupported(under.Elem())
		e.litStack = e.litStack[:len(e.litStack)-1]
		return ok

	case *types.Array:
		e.litStack = append(e.litStack, ts)
		ok := e.literalSupported(under.Elem())
		e.litStack = e.litStack[:len(e.litStack)-1]
		return ok

	case *types.Struct:
		e.litStack = append(e.litStack, ts)
		defer func() { e.litStack = e.litStack[:len(e.litStack)-1] }()
		for i := 0; i < under.NumFields(); i++ {
			f := under.Field(i)
			if f.Name() == "_" {
				continue // blank fields are unreachable and always zero
			}
			if !f.Exported() {
				return false
			}
			if !e.literalSupported(f.Type()) {
				return false
			}
		}
		return true

	case *types.Pointer:
		e.litStack = append(e.litStack, ts)
		ok := e.literalSupported(under.Elem())
		e.litStack = e.litStack[:len(e.litStack)-1]
		return ok

	default:
		return false
	}
}

// wrapLiteral wraps inner — a Go expression that evaluates to the plain
// rendering of a basic value — with an explicit conversion to typeName, when
// typeName is non-empty. Go's explicit conversion syntax accepts any
// numeric-, string-, or bool-compatible expression regardless of its own
// static type, so this is always assignable — see the design table's
// "explicit conversion is always assignable" note. typeName == "" returns
// inner unchanged.
func wrapLiteral(typeName, inner string) string {
	if typeName == "" {
		return inner
	}
	return fmt.Sprintf("%q + %s + %q", typeName+"(", inner, ")")
}

// basicLiteralExpr renders the PLAIN (unwrapped) value of a basic-kinded
// expression src as a Go expression string that evaluates, at run time in
// the generated file, to that value's textual form. The caller — via
// literalBasicWrapped — applies wrapLiteral on top when a named type, or an
// unnamed float32 (whose non-finite rendering has concrete type float64, a
// mismatch), needs the assignment to type-check.
func (e *fuzzEmitter) basicLiteralExpr(b *types.Basic, src string) (string, error) {
	e.needsStrconv = true
	switch b.Kind() {
	case types.Bool:
		return fmt.Sprintf("strconv.FormatBool(bool(%s))", src), nil
	case types.Int, types.Int8, types.Int16, types.Int32, types.Int64:
		return fmt.Sprintf("strconv.FormatInt(int64(%s), 10)", src), nil
	case types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64:
		return fmt.Sprintf("strconv.FormatUint(uint64(%s), 10)", src), nil
	case types.Float32, types.Float64:
		e.needsMath = true
		return fmt.Sprintf("ƒ_fuzzlitfloat_%s(float64(%s))", fuzzFanVersion, src), nil
	case types.String:
		return fmt.Sprintf("strconv.Quote(string(%s))", src), nil
	}
	return "", fmt.Errorf("basicLiteralExpr: kind %v has no literal rendering", b.Kind())
}

// literalBasicWrapped renders t — guaranteed by literalSupported to be a
// basic type or a named type over one — as a self-contained Go expression
// string, applying the explicit-conversion wrap a named type (or an unnamed
// float32) needs to type-check at the splice site.
func (e *fuzzEmitter) literalBasicWrapped(t types.Type, src string) (string, error) {
	u := types.Unalias(t)
	if named, ok := u.(*types.Named); ok {
		b, ok := named.Underlying().(*types.Basic)
		if !ok {
			return "", fmt.Errorf("literalBasicWrapped: %s is not a named basic type", e.typeRef(t))
		}
		inner, err := e.basicLiteralExpr(b, src)
		if err != nil {
			return "", err
		}
		return wrapLiteral(e.typeRef(t), inner), nil
	}
	b, ok := u.(*types.Basic)
	if !ok {
		return "", fmt.Errorf("literalBasicWrapped: %s is not a basic type", e.typeRef(t))
	}
	inner, err := e.basicLiteralExpr(b, src)
	if err != nil {
		return "", err
	}
	if b.Kind() == types.Float32 {
		return wrapLiteral("float32", inner), nil
	}
	return inner, nil
}

// literalExpr returns a Go expression that, at run time in the generated
// file, evaluates to src's (of type t) self-contained literal rendering.
// Basic-kinded types (named or not) are always inlined directly, mirroring
// basicRead/basicWrite; composite types route through literalHelper exactly
// as helperRead/helperWrite do, so a composite type used from N call sites
// gets exactly one literal function.
func (e *fuzzEmitter) literalExpr(t types.Type, src string) (string, error) {
	u := types.Unalias(t)

	if named, ok := u.(*types.Named); ok {
		if _, ok := named.Underlying().(*types.Basic); ok {
			return e.literalBasicWrapped(t, src)
		}
		return e.literalHelperCall(t, src)
	}

	switch u.(type) {
	case *types.Basic:
		return e.literalBasicWrapped(t, src)
	case *types.Slice, *types.Struct, *types.Array, *types.Pointer:
		return e.literalHelperCall(t, src)
	}
	return "", fmt.Errorf("literalExpr: %s has no literal rendering", e.typeRef(t))
}

func (e *fuzzEmitter) literalHelperCall(t types.Type, src string) (string, error) {
	name, err := e.literalHelper(t)
	if err != nil {
		return "", err
	}
	return "ƒ_fuzzlit_" + fuzzFanVersion + "_" + name + "(" + src + ")", nil
}

// literalHelper emits the literal-rendering function for a composite type
// (or, at the top level from BuildFuzzCodecs, any supported type) exactly
// once and returns its identifier. The identifier comes from the same
// assignName registry helper/helperRead/helperWrite use, so a type's
// read/write/literal helpers always share one suffix — e.g. Request's
// decoder is ƒ_fuzzdec_v1_Request and its literal function is
// ƒ_fuzzlit_v1_Request.
func (e *fuzzEmitter) literalHelper(t types.Type) (string, error) {
	t = types.Unalias(t)
	ts := types.TypeString(t, e.qual)
	for _, s := range e.litStack {
		if s == ts {
			return "", fmt.Errorf("type %s is recursive — recursive types are not supported", ts)
		}
	}
	name := e.assignName(ts)
	if _, done := e.literals[name]; done {
		return name, nil
	}

	e.litStack = append(e.litStack, ts)
	src, err := e.literalHelperSource(t, ts, name)
	e.litStack = e.litStack[:len(e.litStack)-1]
	if err != nil {
		return "", err
	}

	e.literals[name] = src
	e.literalOrder = append(e.literalOrder, name)
	return name, nil
}

// literalHelperSource builds the func(typeRef) string body for one type,
// mirroring helperSource's switch over struct/slice/array/pointer — plus a
// basic case, since literalHelper (unlike helper) is also the entry point
// BuildFuzzCodecs uses directly for a top-level type that turns out to be a
// bare basic or a named type over one (e.g. fuzzing a `type Priority int`
// argument directly, with no enclosing struct).
func (e *fuzzEmitter) literalHelperSource(t types.Type, typeRef, name string) (string, error) {
	funcName := "ƒ_fuzzlit_" + fuzzFanVersion + "_" + name
	var b strings.Builder
	fmt.Fprintf(&b, "\nfunc %s(ƒv %s) string {\n", funcName, typeRef)

	u := types.Unalias(t)
	var underlying types.Type = u
	if named, ok := u.(*types.Named); ok {
		underlying = named.Underlying()
	}

	switch under := underlying.(type) {
	case *types.Basic:
		expr, err := e.literalBasicWrapped(t, "ƒv")
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "\treturn %s\n", expr)

	case *types.Struct:
		e.needsStrings = true
		b.WriteString("\tvar ƒb strings.Builder\n")
		fmt.Fprintf(&b, "\tƒb.WriteString(%q)\n", typeRef+"{")
		wrote := false
		for i := 0; i < under.NumFields(); i++ {
			f := under.Field(i)
			if f.Name() == "_" || !f.Exported() {
				continue // unreachable (blank) or already rejected upstream
			}
			if wrote {
				b.WriteString("\tƒb.WriteString(\", \")\n")
			}
			wrote = true
			fmt.Fprintf(&b, "\tƒb.WriteString(%q)\n", f.Name()+": ")
			expr, err := e.literalExpr(f.Type(), "ƒv."+f.Name())
			if err != nil {
				return "", err
			}
			fmt.Fprintf(&b, "\tƒb.WriteString(%s)\n", expr)
		}
		b.WriteString("\tƒb.WriteString(\"}\")\n")
		b.WriteString("\treturn ƒb.String()\n")

	case *types.Slice:
		b.WriteString("\tif ƒv == nil {\n\t\treturn \"nil\"\n\t}\n")
		if isUnnamedByte(under.Elem()) {
			e.needsStrconv = true
			fmt.Fprintf(&b, "\treturn %q + strconv.Quote(string(ƒv)) + %q\n", typeRef+"(", ")")
		} else {
			e.needsStrings = true
			b.WriteString("\tvar ƒb strings.Builder\n")
			fmt.Fprintf(&b, "\tƒb.WriteString(%q)\n", typeRef+"{")
			b.WriteString("\tfor ƒi := range ƒv {\n")
			b.WriteString("\t\tif ƒi > 0 {\n\t\t\tƒb.WriteString(\", \")\n\t\t}\n")
			expr, err := e.literalExpr(under.Elem(), "ƒv[ƒi]")
			if err != nil {
				return "", err
			}
			fmt.Fprintf(&b, "\t\tƒb.WriteString(%s)\n", expr)
			b.WriteString("\t}\n")
			b.WriteString("\tƒb.WriteString(\"}\")\n")
			b.WriteString("\treturn ƒb.String()\n")
		}

	case *types.Array:
		e.needsStrings = true
		b.WriteString("\tvar ƒb strings.Builder\n")
		fmt.Fprintf(&b, "\tƒb.WriteString(%q)\n", typeRef+"{")
		b.WriteString("\tfor ƒi := range ƒv {\n")
		b.WriteString("\t\tif ƒi > 0 {\n\t\t\tƒb.WriteString(\", \")\n\t\t}\n")
		expr, err := e.literalExpr(under.Elem(), "ƒv[ƒi]")
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "\t\tƒb.WriteString(%s)\n", expr)
		b.WriteString("\t}\n")
		b.WriteString("\tƒb.WriteString(\"}\")\n")
		b.WriteString("\treturn ƒb.String()\n")

	case *types.Pointer:
		b.WriteString("\tif ƒv == nil {\n\t\treturn \"nil\"\n\t}\n")
		expr, err := e.literalExpr(under.Elem(), "*ƒv")
		if err != nil {
			return "", err
		}
		if isLiteralStructShape(under.Elem()) {
			// "&T{...}" reads better than the slice-index form and is valid
			// Go for any struct element, so keep it for pointer-to-struct.
			fmt.Fprintf(&b, "\treturn \"&\" + %s\n", expr)
		} else {
			// No bare "&5" exists in Go, but slice indexing is an
			// addressable operand per the spec, so "&[]T{elem}[0]" is valid
			// on every Go version gotest supports (unlike new(expr), which
			// Go 1.26 added but which depends on the SPLICE SITE's module
			// declaring go 1.26+ — not something a codec emitted here can
			// assume). Reusing literalExpr's element rendering means a named
			// basic still gets its explicit conversion, e.g.
			// "&[]Level{Level(3)}[0]".
			elemTypeRef := e.typeRef(under.Elem())
			fmt.Fprintf(&b, "\treturn %q + %s + %q\n", "&[]"+elemTypeRef+"{", expr, "}[0]")
		}

	default:
		return "", fmt.Errorf("literalHelperSource: %s has no literal rendering", typeRef)
	}

	b.WriteString("}\n")
	return b.String(), nil
}
