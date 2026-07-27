package gotestgen

import (
	"fmt"
	"go/types"
	"sort"
	"strings"
	"unicode"

	"github.com/mvrahden/go-test/internal/about"
	"github.com/mvrahden/go-test/internal/gotestast"
	"golang.org/x/tools/go/packages"
)

// fuzzCodecVersion stamps every generated codec identifier. The wire format
// is internal and undocumented on purpose, but it is versioned so a future
// format change can never silently reinterpret a cached corpus: the
// identifiers move, the generated file changes, and the old bytes are read
// by nothing.
const fuzzCodecVersion = "v1"

// fuzzCodecRuntimeImport is the import path every emitted codec needs.
var fuzzCodecRuntimeImport = about.Repo + "/pkg/gotestruntime"

// FuzzCodecRef names one generated codec, as the NewF call in the fuzz
// wrapper needs to reference it.
type FuzzCodecRef struct {
	TypeRef    string // Go type expression valid in the generated file, e.g. "Request" or "user.Request"
	DecodeFunc string // e.g. "ƒ_fuzzdec_v1_Request"
	EncodeFunc string // e.g. "ƒ_fuzzenc_v1_Request"
}

// FuzzCodecSet is everything the renderer needs to emit struct-fuzzing
// support for one generated file.
type FuzzCodecSet struct {
	Codecs   []FuzzCodecRef // one per non-native type the package fuzzes, sorted by TypeRef
	Source   string         // deduplicated source of every decoder/encoder and helper
	PkgPaths []string       // import paths Source references, excluding gotestruntime
}

// BuildFuzzCodecs resolves every non-native fuzz argument type in pkg and
// emits a total decoder/encoder for it. Returns (nil, nil) when every fuzz
// target already uses one of Go's native fuzzable types.
//
// Rejections are errors, not silent skips. Nothing in the toolchain catches
// this for us: go vet only checks direct (*testing.F).Fuzz calls, and the
// generic adapter hides the instantiation from it, so an unsupported type
// compiles cleanly and panics at run time with "unsupported type for
// fuzzing". Refusing at generation time is the only place a useful message
// can be produced.
func BuildFuzzCodecs(pkg *packages.Package, suites gotestast.TestSuiteSpecSet) (*FuzzCodecSet, error) {
	args := gotestast.CollectFuzzArgs(pkg, suites)
	if len(args) == 0 {
		return nil, nil
	}

	e := newFuzzEmitter(pkg)

	// Collect the distinct non-native types first, so emission order (and
	// therefore helper naming) depends only on the type set, not on which
	// target happened to mention a type first.
	type pending struct {
		typ      types.Type
		typeRef  string
		funcName string
	}
	seen := map[string]bool{}
	var todo []pending
	for _, a := range args {
		if nativeFuzzType(a.Type) {
			continue
		}
		if a.Adapter != "Fuzz" {
			return nil, fmt.Errorf(
				"fuzz target %s: gotest.%s argument %d has type %s — multi-argument fuzz targets support only Go's native fuzzing types; wrap them in a single struct and use gotest.Fuzz",
				a.FuncName, a.Adapter, a.Index, types.TypeString(a.Type, e.qual))
		}
		// Unaliased, so "type Alias = Inner" and Inner resolve to one codec.
		// They are the same type, so Codec[Alias] and Codec[Inner] are the
		// same instantiation — emitting both would put two interchangeable
		// codecs on every F and make seed attribution ambiguous.
		typ := types.Unalias(a.Type)
		ref := types.TypeString(typ, e.qual)
		if seen[ref] {
			continue
		}
		seen[ref] = true
		todo = append(todo, pending{typ: typ, typeRef: ref, funcName: a.FuncName})
	}
	if len(todo) == 0 {
		return nil, nil
	}
	sort.Slice(todo, func(i, j int) bool { return todo[i].typeRef < todo[j].typeRef })

	var body strings.Builder
	var refs []FuzzCodecRef
	for i := range todo {
		p := &todo[i]
		ident := e.assignName(p.typeRef)
		decName := "ƒ_fuzzdec_" + fuzzCodecVersion + "_" + ident
		encName := "ƒ_fuzzenc_" + fuzzCodecVersion + "_" + ident

		e.path = []string{p.typeRef}
		readExpr, err := e.readCall(p.typ)
		if err != nil {
			return nil, fmt.Errorf("fuzz target %s: %w", p.funcName, err)
		}
		writeStmt, err := e.writeStmt(p.typ, "ƒv")
		if err != nil {
			return nil, fmt.Errorf("fuzz target %s: %w", p.funcName, err)
		}

		fmt.Fprintf(&body, "\nfunc %s(ƒb []byte) %s {\n\tƒr := gotestruntime.NewFuzzReader(ƒb)\n\treturn %s\n}\n",
			decName, p.typeRef, readExpr)
		fmt.Fprintf(&body, "\nfunc %s(ƒv %s) []byte {\n\tƒw := gotestruntime.NewFuzzWriter()\n\t%s\n\treturn ƒw.Out()\n}\n",
			encName, p.typeRef, writeStmt)

		refs = append(refs, FuzzCodecRef{TypeRef: p.typeRef, DecodeFunc: decName, EncodeFunc: encName})
	}

	var src strings.Builder
	for _, name := range e.order {
		src.WriteString(e.helpers[name])
	}
	src.WriteString(body.String())

	pkgPaths := make([]string, 0, len(e.pkgPaths))
	for path := range e.pkgPaths {
		pkgPaths = append(pkgPaths, path)
	}
	sort.Strings(pkgPaths)

	return &FuzzCodecSet{Codecs: refs, Source: src.String(), PkgPaths: pkgPaths}, nil
}

// nativeFuzzType reports whether Go's fuzzing engine accepts t directly.
// The set is exactly the fifteen types testing.F.Fuzz allows; a named type
// over one of them does NOT qualify (testing matches on reflect.Type
// identity), which is why "type Age int" needs a codec just as a struct
// does.
func nativeFuzzType(t types.Type) bool {
	switch u := types.Unalias(t).(type) {
	case *types.Basic:
		switch u.Kind() {
		case types.String, types.Bool,
			types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
			types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64,
			types.Float32, types.Float64:
			return true
		}
	case *types.Slice:
		return isUnnamedByte(u.Elem())
	}
	return false
}

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
}

func newFuzzEmitter(pkg *packages.Package) *fuzzEmitter {
	e := &fuzzEmitter{
		genPkg:   pkg.Types,
		pkgPaths: map[string]bool{},
		idents:   map[string]string{},
		taken:    map[string]bool{},
		helpers:  map[string]string{},
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
	return "ƒ_fuzzread_" + fuzzCodecVersion + "_" + name + "(ƒr)", nil
}

func (e *fuzzEmitter) helperWrite(t types.Type, src string) (string, error) {
	name, err := e.helper(t)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("ƒ_fuzzwrite_%s_%s(ƒw, %s)", fuzzCodecVersion, name, src), nil
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
	readName := "ƒ_fuzzread_" + fuzzCodecVersion + "_" + name
	writeName := "ƒ_fuzzwrite_" + fuzzCodecVersion + "_" + name

	var read, write strings.Builder
	fmt.Fprintf(&read, "\nfunc %s(ƒr *gotestruntime.FuzzReader) %s {\n", readName, typeRef)
	fmt.Fprintf(&write, "\nfunc %s(ƒw *gotestruntime.FuzzWriter, ƒv %s) {\n", writeName, typeRef)

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

// isUnnamedByte reports whether t is exactly the predeclared byte/uint8 —
// the element type that makes a slice encodable as a length-prefixed blob. A
// named byte type is not, since ƒr.ByteSlice() would yield the wrong slice
// type.
func isUnnamedByte(t types.Type) bool {
	b, ok := types.Unalias(t).(*types.Basic)
	return ok && b.Kind() == types.Uint8
}
