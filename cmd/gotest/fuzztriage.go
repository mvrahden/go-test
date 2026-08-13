package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/internal/refactor"
)

// extractFuzzSubcommand reports whether args begins with the "triage" or
// "promote" subcommand. The grammar is strictly positional — `gotest fuzz
// triage [packages...]` — because anything looser has to guess: a previous
// scan-for-the-first-non-flag version read `gotest fuzz --for=5m triage` as
// triage (silently dropping the flag) and `gotest fuzz ./... triage` as a
// fuzz run (silently treating "triage" as a package pattern). A subcommand
// word anywhere past the first position is rejected loudly by
// misplacedFuzzSubcommand instead of being reinterpreted either way.
func extractFuzzSubcommand(args []string) (sub string, rest []string, ok bool) {
	if len(args) == 0 || (args[0] != "triage" && args[0] != "promote") {
		return "", nil, false
	}
	return args[0], args[1:], true
}

// misplacedFuzzSubcommand returns the first bare "triage"/"promote" word
// found past position 0, or "" when there is none. runFuzz turns a match
// into a usage error: a misplaced subcommand is always a mistake, and both
// silent readings of it (package pattern or dropped word) start a fuzz run
// the user did not ask for.
func misplacedFuzzSubcommand(args []string) string {
	for i, a := range args {
		if i == 0 {
			continue
		}
		if a == "triage" || a == "promote" {
			return a
		}
	}
	return ""
}

// rejectFuzzSubcommandFlags returns an error when args carries any flag.
// triage and promote take package patterns only; accepting-and-ignoring a
// flag (the previous behavior) silently discarded whatever the user thought
// it did.
func rejectFuzzSubcommandFlags(sub string, args []string) error {
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			return fmt.Errorf("fuzz %s takes no flags, only package patterns: unexpected %q", sub, a)
		}
	}
	return nil
}

// corpusArg is one decoded argument from a Go fuzz corpus file
// (testdata/fuzz/<Func>/<hash>), restricted to Go's native primitive corpus
// types (string, []byte, bool, and the int/uint/float variants) — struct-typed
// fuzz callbacks are out of scope (no ƒ_fuzzenc/dec codecs exist).
//
// SourceExpr is always the VERBATIM source text of the value as it appeared
// in the corpus file (e.g. `"a@\x00"`, `-3`) — never reconstructed by
// round-tripping through a decoded value — so both triage's display output
// and promote's splice never risk subtly rewriting the original bytes.
type corpusArg struct {
	TypeName   string
	SourceExpr string
}

// supportedCorpusTypes is the set of Go identifiers understood as native
// fuzz corpus type names (aside from the []byte special case, handled
// separately since it's a composite type, not an identifier).
var supportedCorpusTypes = map[string]bool{
	"string": true, "bool": true,
	"int": true, "int8": true, "int16": true, "int32": true, "int64": true,
	"uint": true, "uint8": true, "uint16": true, "uint32": true, "uint64": true,
	"float32": true, "float64": true,
	"rune": true, "byte": true,
}

// parseCorpusFile parses a Go fuzz corpus file: a "go test fuzz v1" header
// line followed by one "Type(value)" line per fuzz-callback argument. It
// returns an error for a missing/invalid header, or as soon as it hits a
// line it doesn't recognize as a supported primitive type conversion —
// callers are expected to report that error and skip the whole file, per
// the documented per-file-graceful-skip triage/promote behavior.
func parseCorpusFile(path string) ([]corpusArg, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "go test fuzz v1" {
		return nil, fmt.Errorf("missing or invalid corpus header")
	}

	var args []corpusArg
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

// parseCorpusArgLine parses a single "Type(value)" corpus line. It parses
// the line as a Go expression (a type-conversion call is valid Go syntax)
// purely to validate its shape and to locate the value's exact byte range —
// the extracted SourceExpr is always a direct slice of the original line
// text, never a reformatted/reconstructed rendering.
func parseCorpusArgLine(line string) (corpusArg, error) {
	fset := token.NewFileSet()
	expr, err := parser.ParseExprFrom(fset, "", []byte(line), 0)
	if err != nil {
		return corpusArg{}, err
	}
	ce, ok := expr.(*ast.CallExpr)
	if !ok || len(ce.Args) != 1 || ce.Ellipsis != token.NoPos {
		return corpusArg{}, fmt.Errorf("not a recognized Type(value) corpus entry")
	}

	typeName, ok := corpusTypeNameOf(ce.Fun)
	if !ok {
		return corpusArg{}, fmt.Errorf("unsupported corpus type")
	}

	startOff := fset.Position(ce.Args[0].Pos()).Offset
	endOff := fset.Position(ce.Args[0].End()).Offset
	if startOff < 0 || endOff > len(line) || startOff > endOff {
		return corpusArg{}, fmt.Errorf("could not extract corpus value")
	}

	return corpusArg{TypeName: typeName, SourceExpr: line[startOff:endOff]}, nil
}

// corpusTypeNameOf reports the corpus type name a type-conversion call's
// Fun expression names, if it's one Go's native fuzz corpus format supports.
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

// display renders the arg the way it appears in the corpus file, e.g.
// `string("a@\x00")` — used for triage's "input:" line.
func (a corpusArg) display() string {
	return a.TypeName + "(" + a.SourceExpr + ")"
}

// spliceExpr renders the arg the way it should appear inside a spliced
// `f.Add(...)` call. For string and bool, Go's own untyped-constant
// defaulting already produces the exact declared type, so the bare literal
// is used (matching how such seeds are idiomatically hand-written, e.g.
// `f.Add("stale")` rather than `f.Add(string("stale"))`). Every other
// primitive type keeps its explicit conversion, since an untyped literal's
// default type (e.g. plain `5` defaults to `int`, not `int64`) would
// otherwise silently change the argument's static type.
func (a corpusArg) spliceExpr() string {
	if a.TypeName == "string" || a.TypeName == "bool" {
		return a.SourceExpr
	}
	return a.display()
}

// displayArgs joins a crasher's decoded arguments for the triage "input:"
// line.
func displayArgs(args []corpusArg) string {
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = a.display()
	}
	return strings.Join(parts, ", ")
}

// loadFuzzOverlayTargets mirrors runFuzz's package loading + overlay
// generation, returning the generated overlay (needed for triage's re-run
// and promote's suite/method reverse lookup) and the flattened, sorted list
// of fuzz targets.
func loadFuzzOverlayTargets(args []string) (*gotestrunner.OverlayResult, []gotestrunner.FuzzTarget, func(), error) {
	patterns := ExtractPackagePatterns(args)
	loaded, broken, err := gotestgen.LoadPackages(patterns, nil)
	if err != nil {
		return nil, nil, nil, err
	}
	if len(broken) > 0 {
		return nil, nil, nil, fmt.Errorf("cannot triage crashers in uncompilable packages: %s", broken[0].PkgPath)
	}
	overlay, cleanup, err := gotestrunner.GenerateOverlay(loaded, broken, false, false, false)
	if err != nil {
		return nil, nil, nil, err
	}
	return overlay, collectFuzzTargets(overlay), cleanup, nil
}

// crasherFilesFor scans target's testdata/fuzz/<Func>/ directory (LOCKED
// discovery strategy: a plain filesystem scan of the loaded package's source
// dir, not go/packages) for corpus entries, returning their paths sorted for
// determinism. A missing directory (no crashers ever recorded) is not an
// error — it just yields an empty slice.
func crasherFilesFor(target gotestrunner.FuzzTarget) ([]string, error) {
	dir := filepath.Join(target.Dir, "testdata", "fuzz", target.Func)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		files = append(files, filepath.Join(dir, e.Name()))
	}
	sort.Strings(files)
	return files, nil
}

func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// runFuzzTriage re-runs every crasher file found for every discovered fuzz
// target and reports whether it still fails. Exit code is 0 when no
// crashers exist, or when every crasher found turns out to no longer be
// failing; 1 when at least one crasher's re-run still fails, OR when a
// target's testdata/fuzz/<Func>/ directory couldn't even be scanned (e.g.
// permission denied) — that's a real, unaddressed problem, not silently
// "no crashers here", so it must not be allowed to pass as exit 0. Mirrors
// runFuzzPromote's equivalent `failed` handling below.
func runFuzzTriage(args []string) int {
	if err := rejectFuzzSubcommandFlags("triage", args); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	overlay, targets, cleanup, err := loadFuzzOverlayTargets(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	defer cleanup()

	anyCrashers := false
	anyStillFailing := false
	failed := false

	for _, target := range targets { //nolint:gocritic // rangeValCopy: intentional
		files, err := crasherFilesFor(target)
		if err != nil {
			fmt.Fprintf(os.Stderr, "triage: %s: %s\n", target.Func, err)
			failed = true
			continue
		}
		if len(files) == 0 {
			continue
		}
		anyCrashers = true
		fmt.Printf("%s: %d crasher%s\n", target.Func, len(files), pluralS(len(files)))

		for _, file := range files {
			corpusArgs, perr := parseCorpusFile(file)
			if perr != nil {
				// A crasher that cannot even be read is a failure, exactly
				// as promote treats it — exiting 0 here would let a
				// directory of unreadable crashers "pass" triage.
				fmt.Printf("triage: %s: %s\n", file, perr)
				failed = true
				continue
			}

			fmt.Printf("  file:  %s\n", displayPath(file))

			out, code := rerunCrasher(overlay, target, filepath.Base(file))
			if lit := extractDecodedInput(out); lit != "" {
				fmt.Printf("  input: %s\n", lit)
			} else {
				fmt.Printf("  input: %s\n", displayArgs(corpusArgs))
			}
			if code != 0 {
				anyStillFailing = true
				fmt.Printf("  cause: %s\n", extractCause(out))
			} else {
				fmt.Println("  status: no longer failing")
			}
		}
	}

	if !anyCrashers && !failed {
		fmt.Println("no crashers found")
		return 0
	}
	if anyStillFailing || failed {
		return 1
	}
	return 0
}

// rerunCrasher re-runs a single corpus entry as an ordinary (non-fuzzing)
// subtest via `go test -run='^<Func>/<hashBasename>$'` against the
// generated overlay, returning its combined output and exit code.
func rerunCrasher(overlay *gotestrunner.OverlayResult, target gotestrunner.FuzzTarget, hashBase string) (string, int) { //nolint:gocritic // hugeParam: stable API
	goArgs := []string{"test"}
	if overlay.OverlayFlag != "" {
		goArgs = append(goArgs, overlay.OverlayFlag)
	}
	pattern := "^" + regexp.QuoteMeta(target.Func) + "/" + regexp.QuoteMeta(hashBase) + "$"
	// -count=1 disables go test's result cache: a passing (stale) crasher
	// would otherwise be served straight from cache on repeated triage runs,
	// which replays no output at all — silently dropping the very
	// FuzzInputPrefix marker line this re-run exists to capture.
	//
	// -v is required for the same reason on a PASSING replay: the testing
	// package only flushes a fuzz corpus replay's captured output (which is
	// where the FuzzInputPrefix marker line written to stderr ends up) when
	// the subtest fails OR when running verbose. Without it, a no-longer-
	// failing (stale) struct crasher would decode fine but print nothing.
	goArgs = append(goArgs, "-run="+pattern, "-count=1", "-v", target.Package)

	cmd := exec.Command("go", goArgs...) //nolint:gosec // G204: go tool with controlled arguments
	cmd.Dir = target.Dir
	// Echo the decoded input on every execution, not just on failure — this
	// is what makes a no-longer-failing (stale) crasher still show its
	// decoded struct rather than printing no marker line at all.
	cmd.Env = append(os.Environ(), protocol.EnvFuzzEchoInput+"=1")
	out, err := cmd.CombinedOutput()

	code := 0
	if cmd.ProcessState != nil {
		code = cmd.ProcessState.ExitCode()
	} else if err != nil {
		code = 1
	}
	return string(out), code
}

// extractDecodedInput scans a re-run's combined output for
// protocol.FuzzInputPrefix lines and returns the literal carried by the
// LAST one (a failing execution may run the callback body more than once,
// e.g. under -race retries, so the last line is the one describing the
// execution whose outcome the caller is reporting). Returns "" when no such
// line is present — the target isn't struct-typed, or the marker was lost
// (a truncated or filtered run), in which case the caller falls back to the
// raw corpus display. Codec shape is no longer a reason: every type the
// codec generator accepts also gets a Literal function.
func extractDecodedInput(output string) string {
	lit := ""
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if rest, ok := strings.CutPrefix(line, protocol.FuzzInputPrefix); ok {
			lit = rest
		}
	}
	return lit
}

// extractCause pulls the first "panic:" or "--- FAIL" line out of a failed
// re-run's output, for a compact one-line triage summary.
func extractCause(output string) string {
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "panic:") {
			return trimmed
		}
		if strings.Contains(trimmed, "--- FAIL") {
			return trimmed
		}
	}
	return "(see full go test output; no panic/FAIL line found)"
}

// displayPath renders p relative to the current working directory when
// possible, for more readable triage/promote output.
func displayPath(p string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return p
	}
	rel, err := filepath.Rel(cwd, p)
	if err != nil {
		return p
	}
	return rel
}

// lookupSuiteMethod reverse-looks-up the generated Fuzz<Suite>_<Method>
// wrapper name funcName within pkg's entry in overlay.FuzzFuncsByPkg,
// returning the originating suite type name and fuzz method name.
func lookupSuiteMethod(overlay *gotestrunner.OverlayResult, pkg, funcName string) (suite, method string, ok bool) {
	for s, funcs := range overlay.FuzzFuncsByPkg[pkg] {
		for _, fn := range funcs {
			if fn != funcName {
				continue
			}
			prefix := "Fuzz" + s + "_"
			if !strings.HasPrefix(funcName, prefix) {
				return "", "", false
			}
			return s, strings.TrimPrefix(funcName, prefix), true
		}
	}
	return "", "", false
}

// runFuzzPromote splices every crasher file found for every discovered fuzz
// target into its originating fuzz method as a permanent f.Add(...) seed,
// then deletes the crasher file. Exit code is 1 if any crasher could not be
// parsed or promoted (the file is left in place so it isn't silently lost);
// 0 otherwise, including when no crashers exist.
func runFuzzPromote(args []string) int {
	if err := rejectFuzzSubcommandFlags("promote", args); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	overlay, targets, cleanup, err := loadFuzzOverlayTargets(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	defer cleanup()

	anyCrashers := false
	failed := false

	for _, target := range targets { //nolint:gocritic // rangeValCopy: intentional
		files, err := crasherFilesFor(target)
		if err != nil {
			fmt.Fprintf(os.Stderr, "promote: %s: %s\n", target.Func, err)
			failed = true
			continue
		}
		if len(files) == 0 {
			continue
		}
		anyCrashers = true

		suite, method, ok := lookupSuiteMethod(overlay, target.Package, target.Func)
		if !ok {
			fmt.Printf("promote: %s: skipped: could not resolve originating suite/method\n", target.Func)
			failed = true
			continue
		}

		for _, file := range files {
			msg, ok := promoteCrasher(overlay, target, suite, method, file)
			fmt.Println(msg)
			if !ok {
				failed = true
			}
		}
	}

	if !anyCrashers && !failed {
		fmt.Println("no crashers found")
	}
	if failed {
		return 1
	}
	return 0
}

// promoteCrasher processes a single crasher file once its originating
// suite/method is already resolved: it decodes the corpus file and hands
// the splice-ready argument expressions to refactor.PromoteFuzzSeed, which
// owns the "never corrupt user code" contract — if it can't locate the fuzz
// method's *gotest.F parameter, its body, or an insertion point with
// confidence, it returns an error and leaves every file untouched. On any
// failure (decode or splice), the crasher file is deliberately left in
// place (never silently lost) and ok=false is returned so the caller can
// fail the run. Only once the splice has actually landed on disk is the
// crasher file removed.
//
// Extracted out of runFuzzPromote's loop so this confident-skip contract —
// "skipped: ..." printed, crasher file survives — is directly unit
// testable without a full go/packages load.
func promoteCrasher(overlay *gotestrunner.OverlayResult, target gotestrunner.FuzzTarget, suite, method, file string) (message string, ok bool) { //nolint:gocritic // hugeParam: stable API
	hashBase := filepath.Base(file)

	corpusArgs, perr := parseCorpusFile(file)
	if perr != nil {
		return fmt.Sprintf("promote: %s: %s", file, perr), false
	}

	spliceArgs := make([]string, len(corpusArgs))
	for i, a := range corpusArgs {
		spliceArgs[i] = a.spliceExpr()
	}
	// A struct target's corpus entry is an opaque []byte. Re-run it with input
	// echo on and splice the decoded literal instead, so the promoted seed is
	// readable source that survives any future change to the wire format.
	// Guarded to single-corpus-argument targets: only those can carry a
	// codec — Fuzz2/Fuzz3 are native-types-only by design (Phase A).
	if len(corpusArgs) == 1 {
		out, _ := rerunCrasher(overlay, target, hashBase)
		if lit := extractDecodedInput(out); lit != "" {
			spliceArgs = []string{lit}
		}
	}

	editedFile, line, err := refactor.PromoteFuzzSeed(target.Dir, suite, method, spliceArgs)
	if err != nil {
		return fmt.Sprintf("promote: %s/%s: skipped: %s", target.Func, hashBase, err), false
	}

	if err := os.Remove(file); err != nil {
		return fmt.Sprintf("promote: %s/%s: warning: seed added but could not remove crasher file: %s", target.Func, hashBase, err), true
	}

	return fmt.Sprintf("promoted %s/%s -> f.Add(%s) in %s:%d", target.Func, hashBase, strings.Join(spliceArgs, ", "), displayPath(editedFile), line), true
}
