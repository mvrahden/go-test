package gotestrunner

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

// CorpusArg is one decoded value from a Go fuzz corpus file
// (testdata/fuzz/<Func>/<hash>), restricted to Go's native primitive corpus
// types (string, []byte, bool, and the int/uint/float variants). For a fanned
// target these are the raw leaves, not the fuzzed value itself.
//
// SourceExpr is always the VERBATIM source text of the value as it appeared in
// the corpus file (e.g. `"a@\x00"`, `-3`) — never reconstructed by
// round-tripping through a decoded value — so a caller that splices it back
// into Go source can never subtly rewrite the original bytes.
type CorpusArg struct {
	TypeName   string
	SourceExpr string
}

// supportedCorpusTypes is the set of Go identifiers understood as native fuzz
// corpus type names (aside from the []byte special case, handled separately
// since it's a composite type, not an identifier).
var supportedCorpusTypes = map[string]bool{
	"string": true, "bool": true,
	"int": true, "int8": true, "int16": true, "int32": true, "int64": true,
	"uint": true, "uint8": true, "uint16": true, "uint32": true, "uint64": true,
	"float32": true, "float64": true,
	"rune": true, "byte": true,
}

// ParseCorpusFile parses a Go fuzz corpus file: a "go test fuzz v1" header
// line followed by one "Type(value)" line per fuzz-callback argument. It
// returns an error for a missing/invalid header, or as soon as it hits a line
// it doesn't recognize as a supported primitive type conversion — callers are
// expected to report that error and skip the whole file, per the documented
// per-file-graceful-skip triage/promote behavior.
func ParseCorpusFile(path string) ([]CorpusArg, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "go test fuzz v1" {
		return nil, fmt.Errorf("missing or invalid corpus header")
	}

	var args []CorpusArg
	for _, raw := range lines[1:] {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		arg, err := parseCorpusArgLine(line)
		if err != nil {
			return nil, fmt.Errorf("unsupported corpus entry: %s", line)
		}
		args = append(args, arg)
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("no corpus arguments found")
	}
	return args, nil
}

// parseCorpusArgLine parses a single "Type(value)" corpus line. It parses the
// line as a Go expression (a type-conversion call is valid Go syntax) purely
// to validate its shape and to locate the value's exact byte range — the
// extracted SourceExpr is always a direct slice of the original line text,
// never a reformatted/reconstructed rendering.
func parseCorpusArgLine(line string) (CorpusArg, error) {
	fset := token.NewFileSet()
	expr, err := parser.ParseExprFrom(fset, "", []byte(line), 0)
	if err != nil {
		return CorpusArg{}, err
	}
	ce, ok := expr.(*ast.CallExpr)
	if !ok || len(ce.Args) != 1 || ce.Ellipsis != token.NoPos {
		return CorpusArg{}, fmt.Errorf("not a recognized Type(value) corpus entry")
	}

	typeName, ok := corpusTypeNameOf(ce.Fun)
	if !ok {
		return CorpusArg{}, fmt.Errorf("unsupported corpus type")
	}

	startOff := fset.Position(ce.Args[0].Pos()).Offset
	endOff := fset.Position(ce.Args[0].End()).Offset
	if startOff < 0 || endOff > len(line) || startOff > endOff {
		return CorpusArg{}, fmt.Errorf("could not extract corpus value")
	}

	return CorpusArg{TypeName: typeName, SourceExpr: line[startOff:endOff]}, nil
}

// corpusTypeNameOf reports the corpus type name a type-conversion call's Fun
// expression names, if it's one Go's native fuzz corpus format supports.
func corpusTypeNameOf(fun ast.Expr) (string, bool) {
	switch f := fun.(type) {
	case *ast.Ident:
		if supportedCorpusTypes[f.Name] {
			return f.Name, true
		}
	case *ast.ArrayType:
		if f.Len == nil {
			if elt, ok := f.Elt.(*ast.Ident); ok && elt.Name == "byte" {
				return "[]byte", true
			}
		}
	}
	return "", false
}

// CorpusMismatch is one recorded corpus entry whose value shape no longer
// matches the target that owns it.
type CorpusMismatch struct {
	Func string   // generated fuzz func name
	File string   // path of the corpus entry, relative to the package directory
	Got  []string // corpus types the entry holds
	Want []string // corpus types the target now takes
}

// Message is the warning to print for one stale entry. It names the drift and
// both ways out, because neither is right in every case: a crasher worth
// keeping becomes a typed seed, and one that only ever mattered to the old
// shape is noise.
func (m *CorpusMismatch) Message() string {
	return fmt.Sprintf("fuzz: %s: %s has %d values of [%s], but the target now takes %d [%s] — it predates a change to the fuzzed type's fields; run gotest fuzz promote to turn it into a typed f.Add seed, or delete it",
		m.Func, m.File, len(m.Got), strings.Join(m.Got, ", "), len(m.Want), strings.Join(m.Want, ", "))
}

// CheckFuzzCorpus compares every corpus entry recorded under
// dir/testdata/fuzz/<funcName> against want — the corpus type of each value
// the target's engine positions now take. A missing directory is no
// mismatch, and neither is an entry that fails to parse: an unreadable entry
// is triage's business to report, and guessing that it is stale would be
// louder than the drift this checks for.
//
// The check exists because the corpus format records only raw primitives, so
// an entry written before a fuzzed struct gained, lost, or reordered a field
// still loads — Go's engine then rejects it with a bare type error naming the
// generated wrapper, which says nothing about the field that moved.
func CheckFuzzCorpus(dir, funcName string, want []string) ([]CorpusMismatch, error) {
	corpusDir := filepath.Join(dir, "testdata", "fuzz", funcName)
	entries, err := os.ReadDir(corpusDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read fuzz corpus dir: %w", err)
	}

	var mismatches []CorpusMismatch
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		args, err := ParseCorpusFile(filepath.Join(corpusDir, e.Name()))
		if err != nil {
			continue
		}
		got := make([]string, len(args))
		for i, a := range args {
			got[i] = a.TypeName
		}
		if slices.Equal(got, want) {
			continue
		}
		mismatches = append(mismatches, CorpusMismatch{
			Func: funcName,
			File: filepath.ToSlash(filepath.Join("testdata", "fuzz", funcName, e.Name())),
			Got:  got,
			Want: want,
		})
	}
	sort.Slice(mismatches, func(i, j int) bool { return mismatches[i].File < mismatches[j].File })
	return mismatches, nil
}

// ReportStaleFuzzCorpora runs the pre-flight over every fuzz target the
// overlay generated and writes one warning per stale entry to w. It is a
// warning, not a verdict: the run still happens, and the engine's own error
// still ends it — this only says which entry it will be and why.
func ReportStaleFuzzCorpora(w io.Writer, overlay *OverlayResult) {
	pkgs := make([]string, 0, len(overlay.FuzzParamsByFunc))
	for pkg := range overlay.FuzzParamsByFunc {
		pkgs = append(pkgs, pkg)
	}
	sort.Strings(pkgs)

	for _, pkg := range pkgs {
		byFunc := overlay.FuzzParamsByFunc[pkg]
		funcs := make([]string, 0, len(byFunc))
		for fn := range byFunc {
			funcs = append(funcs, fn)
		}
		sort.Strings(funcs)
		for _, fn := range funcs {
			reportStaleCorpus(w, overlay.DirsByPkg[pkg], fn, byFunc[fn])
		}
	}
}

// ReportStaleFuzzCorporaFor is ReportStaleFuzzCorpora narrowed to the targets
// a session actually runs — what `gotest fuzz --target` needs, so a stale
// entry belonging to some other target is not reported as if it were about to
// cost this run its budget.
func ReportStaleFuzzCorporaFor(w io.Writer, overlay *OverlayResult, targets []FuzzTarget) {
	for i := range targets {
		reportStaleCorpus(w, targets[i].Dir, targets[i].Func, overlay.FuzzParamsByFunc[targets[i].Package][targets[i].Func])
	}
}

// reportStaleCorpus writes one line per stale entry for a single target. An
// unknown shape (no generated params, e.g. a target the overlay did not
// record) checks nothing: with nothing to compare against, every entry would
// look stale.
func reportStaleCorpus(w io.Writer, dir, funcName string, want []string) {
	if dir == "" || len(want) == 0 {
		return
	}
	mismatches, err := CheckFuzzCorpus(dir, funcName, want)
	if err != nil {
		return
	}
	for i := range mismatches {
		fmt.Fprintln(w, mismatches[i].Message())
	}
}
