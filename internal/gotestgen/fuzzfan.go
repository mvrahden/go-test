package gotestgen

import (
	"fmt"
	"go/types"
	"sort"
	"strings"

	"github.com/mvrahden/go-test/internal/about"
	"github.com/mvrahden/go-test/internal/gotestast"
	"golang.org/x/tools/go/packages"
)

// fuzzRuntimeImport is the import path every emitted fan needs.
var fuzzRuntimeImport = about.Repo + "/pkg/gotestfuzz"

// arrayFanLimit bounds element-wise fanning of a fixed-size array: an array
// whose elements would add more leaves than this rides as one hybrid []byte
// leaf instead, so a [256]int field cannot dilute the mutator's attention
// across 256 positions.
const arrayFanLimit = 16

// FuzzTargetRef is one generated gotest.FuzzTarget, as the NewF call in the
// fuzz wrapper needs to reference it: a complete composite-literal expression.
type FuzzTargetRef struct {
	FuncName string // the wrapper it binds, e.g. FuzzRichFuzzTestSuite_FuzzRich
	Expr     string // e.g. &gotest.FuzzTarget{Signature: "func(*gotest.T, Rich)", Register: ƒ_fuzzreg_v1_FuzzRichFuzzTestSuite_FuzzRich, Explode: ƒ_fuzzexp_v1_FuzzRichFuzzTestSuite_FuzzRich}
}

// FuzzTargetSet is everything the renderer needs to emit fuzz support for
// one generated file, plus the per-target corpus shape the stale-corpus
// pre-flight compares against.
type FuzzTargetSet struct {
	Targets  []FuzzTargetRef // one per fuzz method, in (suite, method) order
	Source   string          // source of every register, explode, fan-in/out, hybrid codec, and literal function
	PkgPaths []string        // import paths Source references, excluding gotest and gotestfuzz

	// NeedsRuntime reports whether Source uses the gotestfuzz leaf helpers;
	// NeedsStrings/NeedsStrconv/NeedsMath whether it uses the corresponding
	// standard library package — pulled in only by literal rendering.
	NeedsRuntime bool
	NeedsStrings bool
	NeedsStrconv bool
	NeedsMath    bool

	// ParamsByFunc maps every generated Fuzz<Suite>_<Method> wrapper name to
	// the corpus type name of each argument the engine sees ("string",
	// "[]byte", "bool"). A corpus entry whose values do not match is stale.
	ParamsByFunc map[string][]string
}

// BuildFuzzTargets resolves the f.Fuzz call of every fuzz method in pkg and
// emits one target per method. Returns (nil, nil) when the package fuzzes
// nothing at all.
//
// Rejections are errors, not silent skips. Nothing in the toolchain catches
// an unfuzzable type for us: go vet only checks direct (*testing.F).Fuzz
// calls, and f.Fuzz takes any, so an unsupported type would compile cleanly
// and panic at run time. Refusing at generation time is the only place a
// useful message can be produced.
func BuildFuzzTargets(pkg *packages.Package, suites gotestast.TestSuiteSpecSet) (*FuzzTargetSet, error) {
	calls, err := gotestast.CollectFuzzCalls(pkg, suites)
	if err != nil {
		return nil, err
	}
	if len(calls) == 0 {
		return nil, nil
	}
	e := newFuzzEmitter(pkg)

	var body strings.Builder
	var refs []FuzzTargetRef
	params := map[string][]string{}
	for _, c := range calls {
		typs := make([]types.Type, len(c.Types))
		for i, t := range c.Types {
			typs[i] = types.Unalias(t)
		}
		expr, ps, err := e.emitTarget(&body, c.FuncName, typs)
		if err != nil {
			return nil, fmt.Errorf("fuzz target %s: %w", c.FuncName, err)
		}
		params[c.FuncName] = ps
		refs = append(refs, FuzzTargetRef{FuncName: c.FuncName, Expr: expr})
	}

	var src strings.Builder
	for _, name := range e.order {
		src.WriteString(e.helpers[name])
	}
	for _, name := range e.hybridOrder {
		src.WriteString(e.hybrids[name])
	}
	for _, name := range e.fanOrder {
		src.WriteString(e.fans[name])
	}
	src.WriteString(body.String())
	if e.needsMath {
		fmt.Fprintf(&src, literalFloatHelperTpl, fuzzFanVersion)
	}
	for _, name := range e.literalOrder {
		src.WriteString(e.literals[name])
	}

	pkgPaths := make([]string, 0, len(e.pkgPaths))
	for path := range e.pkgPaths {
		pkgPaths = append(pkgPaths, path)
	}
	sort.Strings(pkgPaths)

	return &FuzzTargetSet{
		Targets:      refs,
		Source:       src.String(),
		PkgPaths:     pkgPaths,
		NeedsRuntime: strings.Contains(src.String(), "gotestfuzz."),
		NeedsStrings: e.needsStrings,
		NeedsStrconv: e.needsStrconv,
		NeedsMath:    e.needsMath,
		ParamsByFunc: params,
	}, nil
}

// CheckFuzzArgType reports whether gotest can fuzz an argument of type t —
// as a pass-through kind, or through a generated fan. It returns nil when a
// gotest.Fuzz target of this type will generate, and the emitter's own
// rejection error (naming the offending field path and the suggested
// alternative) when it will not. Callers outside the generator (scaffold)
// use this instead of re-deriving the supported set, so their verdicts can
// never drift from what the generator actually accepts.
func CheckFuzzArgType(pkg *packages.Package, t types.Type) error {
	e := newFuzzEmitter(pkg)
	_, err := e.fan(types.Unalias(t))
	return err
}

// emitTarget emits the register and explode functions of one target (and
// the literal function of its tuple, memoised per tuple) and returns the
// gotest.FuzzTarget expression the wrapper hands to NewF, plus the corpus
// type of every leaf the engine sees.
func (e *fuzzEmitter) emitTarget(body *strings.Builder, funcName string, typs []types.Type) (string, []string, error) {
	infos := make([]*fanInfo, len(typs))
	refs := make([]string, len(typs))
	var params []string
	for i, t := range typs {
		fi, err := e.fan(t)
		if err != nil {
			return "", nil, err
		}
		infos[i] = fi
		refs[i] = types.TypeString(t, e.qual)
		params = append(params, fi.params...)
	}
	sig := "func(*gotest.T" + strings.Join(append([]string{""}, refs...), ", ") + ")"
	if len(params) == 0 {
		return "", nil, fmt.Errorf("f.Fuzz over %s has no fuzzable leaves — nothing for the engine to mutate; fuzz the operation's real input", sig)
	}

	regName := "ƒ_fuzzreg_" + fuzzFanVersion + "_" + funcName
	expName := "ƒ_fuzzexp_" + fuzzFanVersion + "_" + funcName

	// Literal: tuple-level, one argument per declared position, so the echo
	// is a complete f.Add argument list; shared by every target over the
	// same tuple. A bare numeric position is wrapped in its explicit
	// conversion — as an f.Add argument an untyped 7 would default to int
	// and fail the seed type guard.
	// A target whose every position passes through natively gets no literal:
	// its corpus files already read as Go, and triage shows them verbatim.
	literalName := ""
	allLiteral := false
	for _, fi := range infos {
		if !fi.passthrough {
			allLiteral = true
		}
	}
	for _, t := range typs {
		if !e.literalSupported(t) {
			allLiteral = false
		}
	}
	var argNames, sigParams []string
	for i := range typs {
		argNames = append(argNames, fmt.Sprintf("ƒa%d", i))
		sigParams = append(sigParams, fmt.Sprintf("ƒa%d %s", i, refs[i]))
	}
	if allLiteral {
		tupleIdent := infos[0].ident
		if len(typs) > 1 || infos[0].passthrough {
			tupleIdent = e.assignName("(" + strings.Join(refs, ", ") + ")")
		}
		literalName = "ƒ_fuzzlits_" + fuzzFanVersion + "_" + tupleIdent
		if _, done := e.literals[literalName]; !done {
			var lits []string
			for i, t := range typs {
				lit, err := e.topLiteralExpr(t, argNames[i])
				if err != nil {
					return "", nil, err
				}
				lits = append(lits, lit)
			}
			e.literals[literalName] = fmt.Sprintf("\nfunc %s(%s) string {\n\treturn %s\n}\n",
				literalName, strings.Join(sigParams, ", "), strings.Join(lits, ` + ", " + `))
			e.literalOrder = append(e.literalOrder, literalName)
		}
	}

	// Register: assert the callback to the exact type the wrapper was
	// generated for, then the direct (*testing.F).Fuzz call with the fanned
	// leaf signature — the shape vet checks and the engine mutates leaf by
	// leaf. Each execution fans the leaves back in and runs under ƒeach.
	var leafParams []string
	for i, p := range params {
		leafParams = append(leafParams, fmt.Sprintf("ƒ%d %s", i, p))
	}
	var fanIn strings.Builder
	next := 0
	for i, fi := range infos {
		names := make([]string, len(fi.params))
		for j := range names {
			names[j] = fmt.Sprintf("ƒ%d", next+j)
		}
		next += len(fi.params)
		fmt.Fprintf(&fanIn, "\t\t%s := %s\n", argNames[i], e.inExpr(typs[i], fi, names, false))
	}
	literalArg := "nil"
	if literalName != "" {
		literalArg = fmt.Sprintf("func() string { return %s(%s) }", literalName, strings.Join(argNames, ", "))
	}
	fmt.Fprintf(body, "\nfunc %s(ƒf *testing.F, ƒfn any, ƒeach gotest.FuzzEach) bool {\n\tƒrun, ok := ƒfn.(%s)\n\tif !ok {\n\t\treturn false\n\t}\n\tƒf.Fuzz(func(ƒt *testing.T, %s) {\n%s\t\tƒeach(ƒt, %s, func(ƒtt *gotest.T) { ƒrun(ƒtt, %s) })\n\t})\n\treturn true\n}\n",
		regName, sig, strings.Join(leafParams, ", "), fanIn.String(), literalArg, strings.Join(argNames, ", "))

	// Explode: one typed f.Add tuple to the same leaves, with the arity and
	// each position's type checked against the declared signature.
	var exp strings.Builder
	fmt.Fprintf(&exp, "\nfunc %s(ƒseed []any) ([]any, error) {\n\tif err := gotest.SeedArity(ƒseed, %d); err != nil {\n\t\treturn nil, err\n\t}\n", expName, len(typs))
	for i, t := range typs {
		fmt.Fprintf(&exp, "\t%s, err := gotest.SeedArg[%s](ƒseed, %d)\n\tif err != nil {\n\t\treturn nil, err\n\t}\n", argNames[i], refs[i], i)
		_ = t
	}
	fmt.Fprintf(&exp, "\tƒo := make([]any, 0, %d)\n", len(params))
	for i, t := range typs {
		fmt.Fprintf(&exp, "\tƒo = append(ƒo, %s...)\n", e.outExpr(t, infos[i], argNames[i]))
	}
	exp.WriteString("\treturn ƒo, nil\n}\n")
	body.WriteString(exp.String())

	expr := fmt.Sprintf("&gotest.FuzzTarget{Signature: %q, Register: %s, Explode: %s}", sig, regName, expName)
	return expr, params, nil
}

// fanInfo describes how one type fans: the corpus type name of each of its
// leaves, in declaration order, and the identifier its fan-in/fan-out
// functions were emitted under. A pass-through type has one leaf and no
// functions — it is handed to the engine as declared.
type fanInfo struct {
	ident       string
	params      []string
	passthrough bool
}

// passthroughParam names the corpus type of a pass-through kind.
func passthroughParam(t types.Type) string {
	if b, ok := types.Unalias(t).(*types.Basic); ok {
		if b.Kind() == types.Bool {
			return "bool"
		}
		return "string"
	}
	return "[]byte"
}

// fan resolves how t fans out, emitting (and memoising) its fan-in and
// fan-out functions and everything they call. Rejections carry the field
// path accumulated in e.path.
func (e *fuzzEmitter) fan(t types.Type) (*fanInfo, error) {
	t = types.Unalias(t)
	if gotestast.PassthroughFuzzType(t) {
		return &fanInfo{params: []string{passthroughParam(t)}, passthrough: true}, nil
	}
	ts := types.TypeString(t, e.qual)
	if fi, ok := e.fanInfos[ts]; ok {
		return fi, nil
	}
	if len(e.path) == 0 {
		// Top of a walk: rejections below name the field path from here.
		e.path = []string{ts}
		defer func() { e.path = nil }()
	}
	for _, s := range e.fanStack {
		if s == ts {
			return nil, fmt.Errorf("type %s is recursive — recursive types are not supported; a depth-limited variant would be needed", ts)
		}
	}
	e.fanStack = append(e.fanStack, ts)
	defer func() { e.fanStack = e.fanStack[:len(e.fanStack)-1] }()

	ident := e.assignName(ts)
	fi := &fanInfo{ident: ident}
	inName := "ƒ_fuzzin_" + fuzzFanVersion + "_" + ident
	outName := "ƒ_fuzzout_" + fuzzFanVersion + "_" + ident
	ref := e.typeRef(t)

	var in, out strings.Builder
	var params []string
	_, isNamed := t.(*types.Named)

	switch u := t.Underlying().(type) {
	case *types.Basic:
		switch {
		case u.Kind() == types.String || u.Kind() == types.Bool:
			// A named string/bool: one native leaf plus the conversion.
			p := passthroughParam(u)
			params = []string{p}
			fmt.Fprintf(&in, "\treturn %s(ƒ0)\n", ref)
			fmt.Fprintf(&out, "\treturn []any{%s(ƒv)}\n", p)
		default:
			m, ok := fuzzBasicMethod[u.Kind()]
			if !ok {
				return nil, e.reject(t, fuzzRejectReason(u))
			}
			// A number, named or not: one fixed-width []byte leaf. Bytes
			// get the engine's richest mutator; the native number kinds
			// get its poorest.
			params = []string{"[]byte"}
			if isNamed {
				fmt.Fprintf(&in, "\treturn %s(gotestfuzz.Leaf%s(ƒ0))\n", ref, m)
			} else {
				fmt.Fprintf(&in, "\treturn gotestfuzz.Leaf%s(ƒ0)\n", m)
			}
			fmt.Fprintf(&out, "\treturn []any{gotestfuzz.LeafBytes%s(%s(ƒv))}\n", m, u.Name())
		}

	case *types.Slice:
		if isUnnamedByte(u.Elem()) {
			// A named byte slice: one native []byte leaf plus the conversion,
			// empty collapsed to nil like every []byte the fan constructs.
			params = []string{"[]byte"}
			fmt.Fprintf(&in, "\treturn %s(gotestfuzz.LeafBytes(ƒ0))\n", ref)
			out.WriteString("\treturn []any{[]byte(ƒv)}\n")
			break
		}
		if err := e.hybrid(&in, &out, t, ref, ident); err != nil {
			return nil, err
		}
		params = []string{"[]byte"}

	case *types.Array:
		n := int(u.Len())
		if isUnnamedByte(u.Elem()) {
			// A byte array: one native []byte leaf, padded or truncated to N.
			params = []string{"[]byte"}
			fmt.Fprintf(&in, "\tvar ƒa %s\n\tcopy(ƒa[:], ƒ0)\n\treturn ƒa\n", ref)
			out.WriteString("\treturn []any{append([]byte(nil), ƒv[:]...)}\n")
			break
		}
		elem, err := e.fan(u.Elem())
		if err != nil {
			return nil, err
		}
		if n > 0 && n*len(elem.params) <= arrayFanLimit {
			// Element-wise: static arity, so each element's leaves are their
			// own corpus values.
			var elems, appends []string
			for i := 0; i < n; i++ {
				names := make([]string, len(elem.params))
				for j := range names {
					names[j] = fmt.Sprintf("ƒ%d", len(params)+j)
				}
				params = append(params, elem.params...)
				elems = append(elems, e.inExpr(u.Elem(), elem, names, true))
				appends = append(appends, fmt.Sprintf("\tƒo = append(ƒo, %s...)\n", e.outExpr(u.Elem(), elem, fmt.Sprintf("ƒv[%d]", i))))
			}
			fmt.Fprintf(&in, "\treturn %s{%s}\n", ref, strings.Join(elems, ", "))
			fmt.Fprintf(&out, "\tƒo := make([]any, 0, %d)\n%s\treturn ƒo\n", len(params), strings.Join(appends, ""))
			break
		}
		if err := e.hybrid(&in, &out, t, ref, ident); err != nil {
			return nil, err
		}
		params = []string{"[]byte"}

	case *types.Struct:
		var fields, appends []string
		for i := 0; i < u.NumFields(); i++ {
			f := u.Field(i)
			if f.Name() == "_" {
				continue // blank fields are unreachable and always zero
			}
			e.path = append(e.path, f.Name())
			if !f.Exported() {
				err := e.reject(f.Type(), "unexported fields cannot be set — fuzz the constructor's input, or declare a local wrapper struct")
				e.path = e.path[:len(e.path)-1]
				return nil, err
			}
			ft := f.Type()
			sub, err := e.fan(ft)
			e.path = e.path[:len(e.path)-1]
			if err != nil {
				return nil, err
			}
			names := make([]string, len(sub.params))
			for j := range names {
				names[j] = fmt.Sprintf("ƒ%d", len(params)+j)
			}
			params = append(params, sub.params...)
			fields = append(fields, f.Name()+": "+e.inExpr(ft, sub, names, true))
			appends = append(appends, fmt.Sprintf("\tƒo = append(ƒo, %s...)\n", e.outExpr(ft, sub, "ƒv."+f.Name())))
		}
		fmt.Fprintf(&in, "\treturn %s{%s}\n", ref, strings.Join(fields, ", "))
		fmt.Fprintf(&out, "\tƒo := make([]any, 0, %d)\n%s\treturn ƒo\n", len(params), strings.Join(appends, ""))

	case *types.Pointer:
		elem, err := e.fan(u.Elem())
		if err != nil {
			return nil, err
		}
		// A bool nil-flag leaf, then the pointee's leaves; a nil pointer
		// still explodes to the full arity, with the pointee's zero value.
		params = append([]string{"bool"}, elem.params...)
		names := make([]string, len(elem.params))
		for j := range names {
			names[j] = fmt.Sprintf("ƒ%d", 1+j)
		}
		elemRef := e.typeRef(u.Elem())
		addr := "&ƒx"
		if isNamed {
			addr = ref + "(&ƒx)"
		}
		fmt.Fprintf(&in, "\tif !ƒ0 {\n\t\treturn nil\n\t}\n\tƒx := %s\n\treturn %s\n", e.inExpr(u.Elem(), elem, names, true), addr)
		fmt.Fprintf(&out, "\tif ƒv == nil {\n\t\tvar ƒz %s\n\t\treturn append([]any{false}, %s...)\n\t}\n\treturn append([]any{true}, %s...)\n",
			elemRef, e.outExpr(u.Elem(), elem, "ƒz"), e.outExpr(u.Elem(), elem, "*ƒv"))

	default:
		return nil, e.reject(t, fuzzRejectReason(u))
	}

	var leafParams []string
	for i, p := range params {
		leafParams = append(leafParams, fmt.Sprintf("ƒ%d %s", i, p))
	}
	src := fmt.Sprintf("\nfunc %s(%s) %s {\n%s}\n", inName, strings.Join(leafParams, ", "), ref, in.String())
	src += fmt.Sprintf("\nfunc %s(ƒv %s) []any {\n%s}\n", outName, ref, out.String())
	e.fans[ident] = src
	e.fanOrder = append(e.fanOrder, ident)

	fi.params = params
	e.fanInfos[ts] = fi
	return fi, nil
}

// hybrid writes the fan-in/fan-out bodies of a type that rides as one
// []byte leaf through the total mini-codec, emitting (and memoising) the
// codec functions themselves. Everything readCall/writeStmt reject stays
// rejected here, field path included.
func (e *fuzzEmitter) hybrid(in, out *strings.Builder, t types.Type, ref, ident string) error {
	decName := "ƒ_fuzzdec_" + fuzzFanVersion + "_" + ident
	encName := "ƒ_fuzzenc_" + fuzzFanVersion + "_" + ident
	if _, done := e.hybrids[ident]; !done {
		readExpr, err := e.readCall(t)
		if err != nil {
			return err
		}
		writeStmt, err := e.writeStmt(t, "ƒv")
		if err != nil {
			return err
		}
		e.hybrids[ident] = fmt.Sprintf("\nfunc %s(ƒb []byte) %s {\n\tƒr := gotestfuzz.NewReader(ƒb)\n\treturn %s\n}\n", decName, ref, readExpr) +
			fmt.Sprintf("\nfunc %s(ƒv %s) []byte {\n\tƒw := gotestfuzz.NewWriter()\n\t%s\n\treturn ƒw.Out()\n}\n", encName, ref, writeStmt)
		e.hybridOrder = append(e.hybridOrder, ident)
	}
	fmt.Fprintf(in, "\treturn %s(ƒ0)\n", decName)
	fmt.Fprintf(out, "\treturn []any{%s(ƒv)}\n", encName)
	return nil
}

// inExpr returns the Go expression that fans the leaf variables args back
// into a value of type t. A pass-through []byte nested inside a fanned
// value collapses empty to nil (see gotestfuzz.LeafBytes); a top-level
// pass-through position is passed exactly as the engine handed it over.
func (e *fuzzEmitter) inExpr(t types.Type, fi *fanInfo, args []string, nested bool) string {
	if fi.passthrough {
		if nested && fi.params[0] == "[]byte" {
			return "gotestfuzz.LeafBytes(" + args[0] + ")"
		}
		return args[0]
	}
	return "ƒ_fuzzin_" + fuzzFanVersion + "_" + fi.ident + "(" + strings.Join(args, ", ") + ")"
}

// outExpr returns a Go expression of type []any holding the leaves of src,
// a value of type t.
func (e *fuzzEmitter) outExpr(t types.Type, fi *fanInfo, src string) string {
	if fi.passthrough {
		return "[]any{" + src + "}"
	}
	return e.outFunc(t, fi) + "(" + src + ")"
}

func (e *fuzzEmitter) outFunc(_ types.Type, fi *fanInfo) string {
	return "ƒ_fuzzout_" + fuzzFanVersion + "_" + fi.ident
}

// topLiteralExpr is literalExpr for a top-level f.Add position: an unnamed
// numeric type gets its explicit conversion so the spliced argument keeps
// the declared type (a bare 7 defaults to int; a bare 2 to int, not
// float64). Named types and float32 are already wrapped by literalExpr;
// string and bool default correctly.
func (e *fuzzEmitter) topLiteralExpr(t types.Type, src string) (string, error) {
	if b, ok := types.Unalias(t).(*types.Basic); ok && b.Info()&types.IsNumeric != 0 && b.Kind() != types.Float32 {
		inner, err := e.basicLiteralExpr(b, src)
		if err != nil {
			return "", err
		}
		return wrapLiteral(b.Name(), inner), nil
	}
	return e.literalExpr(t, src)
}
