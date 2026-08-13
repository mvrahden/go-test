package scaffold

import (
	"fmt"
	"go/format"
	"go/types"
	"sort"
	"strings"
	"text/template"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"golang.org/x/tools/go/packages"
)

// FuzzTarget describes a package-level function targeted by
// "gotest scaffold --fuzz", along with whatever inverse pairing and
// fuzzability analysis was possible for its single parameter.
type FuzzTarget struct {
	PkgName      string
	PkgDir       string // absolute dir for output file placement
	FuncName     string
	ParamType    types.Type
	ParamTypeStr string   // Go-syntax form of ParamType, relative to the target package
	Fuzzable     bool     // gotest can fuzz ParamType — natively, or via a generated codec
	RejectReason string   // the codec emitter's rejection, set iff !Fuzzable
	ZeroLiteral  string   // Go literal for a f.Add(...) seed; "" = no self-contained zero literal
	ExtraImports []string // packages ParamTypeStr references beyond the target package, sorted
	Pair         *InversePair
}

// InversePair describes a same-package function found to be the inverse of
// a fuzz target: FuncName(A) -> (B[, error]) and Pair.Name(B) -> (A[, error]).
type InversePair struct {
	Name              string
	FuncReturnsErr    bool // does the target function return (B, error)?
	InverseReturnsErr bool // does the inverse function return (A, error)?
}

// errorType is the universe "error" interface, used to detect trailing
// error results via types.Identical.
var errorType = types.Universe.Lookup("error").Type()

// bareInverseNames maps a function name to same-package candidate inverse
// names, checked before signature compatibility. Multiple candidates are
// tried in order; the first one that also exists in the package and has a
// compatible signature wins.
var bareInverseNames = map[string][]string{
	"Marshal":   {"Unmarshal"},
	"Unmarshal": {"Marshal"},
	"Encode":    {"Decode"},
	"Decode":    {"Encode"},
	"Parse":     {"Format", "String"},
	"Format":    {"Parse"},
	"String":    {"Parse"},
}

// prefixInversePairs are verb prefixes that also pair on a shared suffix,
// e.g. "ParseJSON" <-> "FormatJSON", "EncodeVarint" <-> "DecodeVarint".
var prefixInversePairs = [][2]string{
	{"Marshal", "Unmarshal"},
	{"Encode", "Decode"},
	{"Parse", "Format"},
}

// inverseNameCandidates returns the same-package function names that might
// be name-symmetric inverses of name, most-likely first, deduplicated.
func inverseNameCandidates(name string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(n string) {
		if n != "" && n != name && !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	for _, n := range bareInverseNames[name] {
		add(n)
	}
	for _, p := range prefixInversePairs {
		if suffix, ok := strings.CutPrefix(name, p[0]); ok && suffix != "" {
			add(p[1] + suffix)
		}
		if suffix, ok := strings.CutPrefix(name, p[1]); ok && suffix != "" {
			add(p[0] + suffix)
		}
	}
	return out
}

// isErrorType reports whether t is exactly the universe "error" interface.
func isErrorType(t types.Type) bool {
	return types.Identical(t, errorType)
}

// resultShape extracts the non-error result type from a signature shaped
// (X) or (X, error); ok is false for any other result shape (0 results, a
// bare error result, or more than 2 results).
func resultShape(sig *types.Signature) (result types.Type, returnsErr, ok bool) {
	results := sig.Results()
	switch results.Len() {
	case 1:
		t := results.At(0).Type()
		if isErrorType(t) {
			return nil, false, false
		}
		return t, false, true
	case 2:
		t0, t1 := results.At(0).Type(), results.At(1).Type()
		if !isErrorType(t1) {
			return nil, false, false
		}
		return t0, true, true
	default:
		return nil, false, false
	}
}

// signaturesInverse reports whether f and g are inverse-shaped:
// f: A -> (B[, error]) and g: B -> (A[, error]), with A and B compared via
// types.Identical (so distinct named types with the same underlying type
// never qualify).
func signaturesInverse(f, g *types.Signature) (fRetErr, gRetErr, ok bool) {
	if f.Variadic() || g.Variadic() || f.Params().Len() != 1 || g.Params().Len() != 1 {
		return false, false, false
	}
	fParam := f.Params().At(0).Type()
	gParam := g.Params().At(0).Type()

	fResult, fErr, fOK := resultShape(f)
	gResult, gErr, gOK := resultShape(g)
	if !fOK || !gOK {
		return false, false, false
	}
	if !types.Identical(fResult, gParam) || !types.Identical(gResult, fParam) {
		return false, false, false
	}
	return fErr, gErr, true
}

// findInversePair searches scope for a same-package function whose name is
// a candidate inverse of targetName and whose signature is compatible with
// fSig per signaturesInverse. Returns nil if none qualifies.
func findInversePair(scope *types.Scope, targetName string, fSig *types.Signature) *InversePair {
	for _, candidateName := range inverseNameCandidates(targetName) {
		obj := scope.Lookup(candidateName)
		if obj == nil {
			continue
		}
		fn, ok := obj.(*types.Func)
		if !ok {
			continue
		}
		gSig, ok := fn.Type().(*types.Signature)
		if !ok || gSig.Recv() != nil {
			continue
		}
		fErr, gErr, ok := signaturesInverse(fSig, gSig)
		if !ok {
			continue
		}
		return &InversePair{Name: candidateName, FuncReturnsErr: fErr, InverseReturnsErr: gErr}
	}
	return nil
}

// nativeFuzzable reports whether t is one of Go's natively fuzzable types
// (the exact set testing.F.Add/Fuzz accept: string, []byte, bool, and the
// int/uint/float variants — byte and rune are the same Basic kinds under
// different names). Named types never qualify, even when their underlying
// type would (struct fuzzing, and fuzzing through a named wrapper, is not
// supported by go test's fuzzer). zero is a ready-to-print Go literal
// suitable for a f.Add(...) seed.
func nativeFuzzable(t types.Type) (zero string, ok bool) {
	switch bt := t.(type) {
	case *types.Basic:
		switch bt.Kind() {
		case types.String:
			return `""`, true
		case types.Bool:
			return "false", true
		case types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
			types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64:
			return "0", true
		case types.Float32, types.Float64:
			return "0", true
		}
	case *types.Slice:
		if eb, ok := bt.Elem().(*types.Basic); ok && eb.Kind() == types.Uint8 {
			return `[]byte("")`, true
		}
	}
	return "", false
}

// IntrospectFuzzTarget loads the package and extracts fuzz-scaffolding
// information for the named package-level function: its single parameter's
// fuzzability, and (when fuzzable) whatever compatible inverse pair a
// name-table + signature search finds.
func IntrospectFuzzTarget(pkgPattern, funcName string) (*FuzzTarget, error) {
	cfg := &packages.Config{
		Mode:  packages.NeedName | packages.NeedTypes | packages.NeedImports | packages.NeedDeps | packages.NeedFiles,
		Tests: false,
	}

	pkgs, err := packages.Load(cfg, pkgPattern)
	if err != nil {
		return nil, fmt.Errorf("failed to load package %q: %w", pkgPattern, err)
	}
	if len(pkgs) == 0 {
		return nil, fmt.Errorf("no packages found for pattern %q", pkgPattern)
	}
	for _, pkg := range pkgs {
		if len(pkg.Errors) > 0 {
			return nil, fmt.Errorf("package %q has errors: %v", pkgPattern, pkg.Errors[0])
		}
	}

	pkg := pkgs[0]
	scope := pkg.Types.Scope()
	obj := scope.Lookup(funcName)
	if obj == nil {
		return nil, fmt.Errorf("function %q not found in package %q", funcName, pkgPattern)
	}
	fn, ok := obj.(*types.Func)
	if !ok {
		return nil, fmt.Errorf("%q is not a function in package %q", funcName, pkgPattern)
	}
	sig, ok := fn.Type().(*types.Signature)
	if !ok || sig.Recv() != nil {
		return nil, fmt.Errorf("%q is not a package-level function in package %q", funcName, pkgPattern)
	}
	if sig.Variadic() || sig.Params().Len() != 1 {
		return nil, fmt.Errorf("%q must take exactly one non-variadic parameter to scaffold a fuzz target (has %d)", funcName, sig.Params().Len())
	}

	paramType := sig.Params().At(0).Type()

	// Render the type relative to the target package (the skeleton lives
	// there — a self-qualified "codec.Config" would not compile inside
	// package codec), collecting every external package the rendering
	// references so the template can import it.
	external := map[string]bool{}
	relQual := func(p *types.Package) string {
		if p == nil || p == pkg.Types {
			return ""
		}
		external[p.Path()] = true
		return p.Name()
	}
	paramTypeStr := types.TypeString(paramType, relQual)
	extraImports := make([]string, 0, len(external))
	for path := range external {
		extraImports = append(extraImports, path)
	}
	sort.Strings(extraImports)

	zero, fuzzable := nativeFuzzable(paramType)
	reject := ""
	if !fuzzable {
		// One source of truth: the codec emitter's own validation decides
		// whether a non-native type generates, so scaffold's verdict can
		// never drift from what `gotest generate` actually accepts.
		if err := gotestgen.CheckFuzzArgType(pkg, paramType); err != nil {
			reject = err.Error()
		} else {
			fuzzable = true
			zero = codecSeedLiteral(paramType, paramTypeStr)
		}
	}

	target := &FuzzTarget{
		PkgName:      pkg.Name,
		PkgDir:       gotestgen.DeterminePkgDir(pkg),
		FuncName:     funcName,
		ParamType:    paramType,
		ParamTypeStr: paramTypeStr,
		Fuzzable:     fuzzable,
		RejectReason: reject,
		ZeroLiteral:  zero,
		ExtraImports: extraImports,
	}
	if fuzzable {
		target.Pair = findInversePair(scope, funcName, sig)
	}
	return target, nil
}

// codecSeedLiteral returns a zero-value Go literal usable as a f.Add seed
// for a codec-backed (non-native) parameter type, or "" when the shape has
// no self-contained zero literal — the skeleton then carries a TODO line
// instead of a seed.
func codecSeedLiteral(t types.Type, ref string) string {
	switch u := t.Underlying().(type) {
	case *types.Struct, *types.Slice, *types.Array:
		return ref + "{}"
	case *types.Basic:
		info := u.Info()
		switch {
		case info&types.IsString != 0:
			return ref + `("")`
		case info&types.IsBoolean != 0:
			return ref + "(false)"
		case info&types.IsNumeric != 0:
			return ref + "(0)"
		}
	}
	return ""
}

var fuzzTemplate = template.Must(template.New("fuzz").ParseFS(templates, "static/scaffold.fuzz.go.tpl"))

// GenerateFuzzScaffold renders a fuzz test suite skeleton for target: a
// round-trip property test when a compatible inverse pair was found, a
// crash-safety skeleton (calls the function, asserts nothing beyond
// "doesn't panic") when no inverse pair was found, or a TODO stub carrying
// the codec emitter's rejection reason when gotest cannot fuzz the
// parameter type at all (neither natively nor via a generated codec).
// status is a human-readable line describing the fallback taken, or ""
// when a full round-trip skeleton was generated.
func GenerateFuzzScaffold(target *FuzzTarget) (src []byte, status string, err error) {
	var body string
	switch {
	case !target.Fuzzable:
		body = notFuzzableBody(target.RejectReason)
		status = fmt.Sprintf("cannot fuzz %s for %s — generated TODO stub: %s", target.ParamTypeStr, target.FuncName, target.RejectReason)
	case target.Pair != nil:
		body = roundTripBody(target.FuncName, target.Pair, target.ParamTypeStr, target.ZeroLiteral)
	default:
		body = crashSafetyBody(target.FuncName, target.ParamTypeStr, target.ZeroLiteral)
		status = fmt.Sprintf("no inverse pair found for %s — generated crash-safety skeleton", target.FuncName)
	}

	data := struct {
		PkgName      string
		SuiteName    string
		FuncName     string
		Body         string
		ExtraImports []string
	}{
		PkgName:      target.PkgName,
		SuiteName:    target.FuncName + "TestSuite",
		FuncName:     target.FuncName,
		Body:         body,
		ExtraImports: target.ExtraImports,
	}

	var buf strings.Builder
	if err := fuzzTemplate.ExecuteTemplate(&buf, "scaffold.fuzz.go.tpl", data); err != nil {
		return nil, "", fmt.Errorf("template execution failed: %w", err)
	}
	formatted, err := format.Source([]byte(buf.String()))
	if err != nil {
		return nil, "", fmt.Errorf("go/format failed: %w", err)
	}
	return formatted, status, nil
}

// roundTripBody renders the gotest.Fuzz callback body for a found,
// fuzzable inverse pair: encode, (optionally) guard the error, decode,
// (optionally) assert no error, then assert the round trip.
func roundTripBody(funcName string, pair *InversePair, paramTypeStr, zero string) string {
	var b strings.Builder
	writeSeed(&b, zero)
	fmt.Fprintf(&b, "\tgotest.Fuzz(f, func(t *gotest.T, in %s) {\n", paramTypeStr)
	if pair.FuncReturnsErr {
		fmt.Fprintf(&b, "\t\tencoded, err := %s(in)\n", funcName)
		b.WriteString("\t\tif err != nil {\n\t\t\treturn\n\t\t}\n")
	} else {
		fmt.Fprintf(&b, "\t\tencoded := %s(in)\n", funcName)
	}
	if pair.InverseReturnsErr {
		fmt.Fprintf(&b, "\t\tdecoded, err := %s(encoded)\n", pair.Name)
		b.WriteString("\t\tgotest.NoError(t, err)\n")
	} else {
		fmt.Fprintf(&b, "\t\tdecoded := %s(encoded)\n", pair.Name)
	}
	b.WriteString("\t\tgotest.Equal(t, in, decoded) // round-trip property\n")
	b.WriteString("\t})\n")
	return b.String()
}

// crashSafetyBody renders a gotest.Fuzz callback that only calls the
// target function — no assertions beyond "doesn't panic" — for use when
// no inverse pair was found.
func crashSafetyBody(funcName, paramTypeStr, zero string) string {
	var b strings.Builder
	writeSeed(&b, zero)
	fmt.Fprintf(&b, "\tgotest.Fuzz(f, func(t *gotest.T, in %s) {\n", paramTypeStr)
	fmt.Fprintf(&b, "\t\t%s(in) // TODO: assert an invariant beyond \"doesn't crash\" (e.g. idempotence)\n", funcName)
	b.WriteString("\t})\n")
	return b.String()
}

// writeSeed emits the f.Add seed line, or a TODO when the parameter's shape
// has no self-contained zero literal to seed with.
func writeSeed(b *strings.Builder, zero string) {
	if zero == "" {
		b.WriteString("\t// TODO: add representative f.Add seeds\n")
		return
	}
	b.WriteString("\t// TODO: seed with representative inputs\n")
	fmt.Fprintf(b, "\tf.Add(%s)\n", zero)
}

// notFuzzableBody renders a TODO stub carrying the codec emitter's
// rejection reason — no f.Add/gotest.Fuzz call, since that would panic at
// run time rather than fail to compile. The reason already names the
// offending field path and the suggested alternative.
func notFuzzableBody(reason string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\t// Cannot fuzz this parameter: %s\n", reason)
	b.WriteString("\t// TODO: apply the suggested alternative, then re-run gotest scaffold --fuzz.\n")
	return b.String()
}
