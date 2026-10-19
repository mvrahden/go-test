package main

import (
	"errors"
	"fmt"
	"go/types"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"

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

// corpusArg is one decoded argument from a Go fuzz corpus file, as triage and
// promote consume it. The parsing lives in gotestrunner, which needs the same
// shape for the stale-corpus pre-flight; this is the local view of it, with
// the display and splice rendering the two subcommands add on top. For a
// fanned target these are the raw leaves; the readable form comes from the
// echo a re-run prints, and this decoding is the fallback display.
type corpusArg gotestrunner.CorpusArg

// parseCorpusFile decodes a Go fuzz corpus file. A file that is not in Go's
// corpus format is an error located at its line, the path as displayed.
func parseCorpusFile(path string) ([]corpusArg, error) {
	parsed, err := gotestrunner.ParseCorpusFile(path)
	if err != nil {
		var ce *gotestrunner.CorpusError
		if errors.As(err, &ce) {
			located := *ce
			located.Path = displayPath(ce.Path)
			return nil, &located
		}
		return nil, err
	}
	args := make([]corpusArg, len(parsed))
	for i, a := range parsed {
		args[i] = corpusArg(a)
	}
	return args, nil
}

// display renders the arg the way it appears in the corpus file, e.g.
// `string("a@\x00")` — used for triage's "input:" line.
func (a corpusArg) display() string {
	return gotestrunner.CorpusArg(a).Corpus()
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
	if !a.Bits && (a.TypeName == "string" || a.TypeName == "bool") {
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
func loadFuzzOverlayTargets(args []string) (*gotestrunner.OverlayResult, []gotestrunner.FuzzTarget, []*gotestgen.LoadResult, func(), error) {
	patterns, err := ExtractPackagePatterns(args)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	loaded, broken, err := gotestgen.LoadPackages(patterns, nil)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if len(broken) > 0 {
		return nil, nil, nil, nil, fmt.Errorf("cannot triage crashers in uncompilable packages: %s", broken[0].PkgPath)
	}
	overlay, cleanup, err := gotestrunner.GenerateOverlay(loaded, broken, false, false, false)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return overlay, collectFuzzTargets(overlay), loaded, cleanup, nil
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
func runFuzzTriage(args []string) (code int) {
	if err := rejectFuzzSubcommandFlags("triage", args); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	overlay, targets, _, cleanup, err := loadFuzzOverlayTargets(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	defer cleanup()

	anyCrashers := false
	anyStillFailing := false
	failed := false

	type triaged struct {
		target gotestrunner.FuzzTarget
		files  []string
	}
	var todo []triaged
	var withCrashers []gotestrunner.FuzzTarget
	for _, target := range targets { //nolint:gocritic // rangeValCopy: intentional
		files, err := crasherFilesFor(target)
		if err != nil {
			fmt.Fprintf(os.Stderr, "triage: %s: %s\n", target.Func, err)
			failed = true
			continue
		}
		if len(files) > 0 {
			todo = append(todo, triaged{target, files})
			withCrashers = append(withCrashers, target)
		}
	}

	// The re-runs read the shared fixtures their suites bind, as under fuzz.
	ctx, stop := shutdownContext()
	defer stop()
	fixtures, err := gotestrunner.StartFuzzFixtures(ctx, overlay, withCrashers, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 1
	}
	defer func() {
		if err := fixtures.Teardown(); err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: shared fixture teardown: %s\n", err)
			code = max(code, 1)
		}
	}()

	for _, tr := range todo { //nolint:gocritic // rangeValCopy: intentional
		target, files := tr.target, tr.files
		anyCrashers = true
		fmt.Printf("%s: %d crasher%s\n", target.Func, len(files), pluralS(len(files)))

		env, never := fixtures.Env(target)
		if never != "" {
			// Nothing was proven about these crashers: unverified, not failing.
			fmt.Printf("  status: unverified — %s\n", never)
			failed = true
			continue
		}
		for _, file := range files {
			corpusArgs, perr := parseCorpusFile(file)
			if perr != nil {
				// A crasher that cannot even be read is a failure, exactly
				// as promote treats it — exiting 0 here would let a
				// directory of unreadable crashers "pass" triage.
				fmt.Printf("triage: %s\n", perr)
				failed = true
				continue
			}

			fmt.Printf("  file:  %s\n", displayPath(file))

			out, status, ran := rerunCrasher(overlay, target, filepath.Base(file), env)
			if lit := extractDecodedInput(out); lit != "" {
				fmt.Printf("  input: %s\n", lit)
			} else {
				fmt.Printf("  input: %s\n", displayArgs(corpusArgs))
			}
			switch state, detail := classifyRerun(status, ran); state {
			case crasherUnverified:
				// Unverified is a triage failure, not a pass: nothing was
				// proven about a crasher whose re-run produced no verdict.
				failed = true
				fmt.Printf("  status: unverified — %s\n", detail)
			case crasherFailing:
				anyStillFailing = true
				fmt.Printf("  cause: %s\n", extractCause(out))
			default:
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

// What a crasher re-run proved.
const (
	crasherFailing    = "failing"
	crasherFixed      = "fixed"
	crasherUnverified = "unverified"
)

// classifyRerun reads what a re-run proved from its exit status. A status the
// process never chose — a signal, or a Windows termination status such as
// 0xC000013A — and a run that never started both prove nothing: the crasher is
// unverified, not still failing, and triage says so instead of printing a
// cause it did not observe.
func classifyRerun(status int, ran bool) (state, detail string) {
	if !ran {
		return crasherUnverified, "the re-run never started"
	}
	code, how := gotestrunner.ExitStatusVerdict(status)
	switch {
	case how != "":
		return crasherUnverified, "the re-run was terminated " + how
	case code != 0:
		return crasherFailing, ""
	}
	return crasherFixed, ""
}

// rerunCrasher re-runs a single corpus entry as an ordinary (non-fuzzing)
// subtest via `go test -run='^<Func>/<hashBasename>$'` against the
// generated overlay, with env added to its environment. It returns the
// combined output, the re-run's exit status and whether it ran at all.
func rerunCrasher(overlay *gotestrunner.OverlayResult, target gotestrunner.FuzzTarget, hashBase string, env []string) (string, int, bool) { //nolint:gocritic // hugeParam: stable API
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
	cmd.Env = append(append(os.Environ(), env...), protocol.EnvFuzzEchoInput+"=1")
	out, _ := cmd.CombinedOutput()

	if cmd.ProcessState == nil {
		return string(out), 0, false
	}
	return string(out), cmd.ProcessState.ExitCode(), true
}

// extractDecodedInput scans a re-run's combined output for
// protocol.FuzzInputPrefix lines and returns the literal carried by the
// FIRST one. The codec echoes the decoded input before the execution's hooks
// and body run, so the first marker is the codec's; one the body prints
// after it, from the input it was handed, is never read. Returns "" when no
// such line is present — the target isn't decoded, or the engine refused the
// entry before the target ran.
func extractDecodedInput(output string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if rest, ok := strings.CutPrefix(line, protocol.FuzzInputPrefix); ok {
			return rest
		}
	}
	return ""
}

// extractCause pulls the reason out of a failed re-run's output for the
// one-line triage summary: a panic line wins; otherwise the first test
// diagnostic (`file.go:12: message`), wherever go test placed it — under
// the FAIL headers, or ahead of them under -v; falling back to the first
// FAIL header when the failure left no message.
func extractCause(output string) string {
	lines := strings.Split(output, "\n")
	header := ""
	diagnostic := ""
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "panic:"):
			return trimmed
		case header == "" && strings.Contains(trimmed, "--- FAIL"):
			header = trimmed
		case diagnostic == "" && diagnosticLine.MatchString(trimmed):
			diagnostic = trimmed
		}
	}
	switch {
	case diagnostic != "":
		return diagnostic
	case header != "":
		return header
	}
	return "(see full go test output; no panic/FAIL line found)"
}

// diagnosticLine matches the "file.go:12: message" shape of testing's own
// output for Errorf/Fatalf/Skipf and every assertion built on them.
var diagnosticLine = regexp.MustCompile(`^[^\s:]+\.go:\d+: `)

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

// runFuzzPromote splices the latest fuzz session's crashers into their
// originating fuzz methods as permanent f.Add(...) seeds, then deletes each
// crasher file. Every other corpus file is left in place. A session crasher
// that is not in Go's corpus format, or changed since the session wrote it,
// refuses the whole run: nothing is promoted and it exits 1. Otherwise it
// exits 1 if any crasher could not be promoted (its file is left in place)
// and 0 when every one was, or there were none.
func runFuzzPromote(args []string) (code int) {
	if err := rejectFuzzSubcommandFlags("promote", args); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	overlay, targets, loaded, cleanup, err := loadFuzzOverlayTargets(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	defer cleanup()

	sessionPath, err := gotestrunner.CurrentFuzzSessionFile()
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	session, _, err := gotestrunner.ReadFuzzSession(sessionPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	type promotion struct {
		target        gotestrunner.FuzzTarget
		suite, method string
		file          string
	}
	var queue []promotion
	failed, refused := false, false

	for _, target := range targets { //nolint:gocritic // rangeValCopy: intentional
		files, err := crasherFilesFor(target)
		if err != nil {
			fmt.Fprintf(os.Stderr, "promote: %s: %s\n", target.Func, err)
			failed = true
			continue
		}
		mine := map[string]gotestrunner.SessionCrasher{}
		for _, c := range session.For(target) {
			mine[c.Name] = c
		}
		suite, method, resolved := lookupSuiteMethod(overlay, target.Package, target.Func)
		for _, file := range files {
			c, ok := mine[filepath.Base(file)]
			if !ok {
				fmt.Printf("promote: %s: left in place: not written by the latest fuzz session\n", displayPath(file))
				continue
			}
			if err := c.Check(); err != nil {
				fmt.Printf("promote: %s: %s\n", displayPath(file), err)
				refused = true
				continue
			}
			if _, err := parseCorpusFile(file); err != nil {
				fmt.Printf("promote: %s\n", err)
				refused = true
				continue
			}
			if !resolved {
				fmt.Printf("promote: %s: skipped: could not resolve originating suite/method\n", target.Func)
				failed = true
				continue
			}
			queue = append(queue, promotion{target: target, suite: suite, method: method, file: file})
		}
	}

	if refused {
		fmt.Println("promote: nothing promoted: a crasher of the latest fuzz session is not one it wrote in Go's corpus format")
		return 1
	}
	if len(queue) == 0 && !failed {
		fmt.Println("no crashers from the latest fuzz session")
		return 0
	}
	// A decoding target's seed comes from a re-run, which reads the shared
	// fixtures its suite binds.
	var rerun []gotestrunner.FuzzTarget
	for _, p := range queue {
		if overlay.FuzzDecodedByFunc[p.target.Package][p.target.Func] {
			rerun = append(rerun, p.target)
		}
	}
	ctx, stop := shutdownContext()
	defer stop()
	fixtures, err := gotestrunner.StartFuzzFixtures(ctx, overlay, rerun, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 1
	}
	defer func() {
		if err := fixtures.Teardown(); err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: shared fixture teardown: %s\n", err)
			code = max(code, 1)
		}
	}()
	for _, p := range queue {
		// A fixture that did not come up leaves the re-run without its state.
		env, _ := fixtures.Env(p.target)
		msg, ok := promoteCrasher(overlay, p.target, p.suite, p.method, p.file, seedTypes(loaded, p.target.Package), env)
		fmt.Println(msg)
		if !ok {
			failed = true
		}
	}
	if failed {
		return 1
	}
	return 0
}

// seedTypes names the types a seed spliced into pkgPath's suites may convert
// to: the predeclared ones, the package's own and those of every package it
// reaches, each by the name it is qualified with.
func seedTypes(loaded []*gotestgen.LoadResult, pkgPath string) refactor.TypeNames {
	var roots []*types.Package
	for _, lr := range loaded {
		if lr.PkgPath != pkgPath {
			continue
		}
		for _, p := range []*packages.Package{lr.Ptest, lr.Pxtest} {
			if p != nil && p.Types != nil {
				roots = append(roots, p.Types)
			}
		}
	}
	isType := func(p *types.Package, name string) bool {
		_, ok := p.Scope().Lookup(name).(*types.TypeName)
		return ok
	}
	return func(qual, name string) bool {
		if qual == "" {
			if _, ok := types.Universe.Lookup(name).(*types.TypeName); ok {
				return true
			}
			for _, r := range roots {
				if isType(r, name) {
					return true
				}
			}
			return false
		}
		seen := map[*types.Package]bool{}
		var reach func(p *types.Package) bool
		reach = func(p *types.Package) bool {
			if seen[p] {
				return false
			}
			seen[p] = true
			if p.Name() == qual && isType(p, name) {
				return true
			}
			for _, imp := range p.Imports() {
				if reach(imp) {
					return true
				}
			}
			return false
		}
		for _, r := range roots {
			if reach(r) {
				return true
			}
		}
		return false
	}
}

// promoteCrasher splices one crasher file of the latest session into its
// originating suite/method, which is already resolved. A target that hands
// the engine its arguments as declared takes the corpus values as written. A
// target that decodes its input from the leaves takes only the value its
// codec decodes, as the re-run echoes it; without that echo the crasher is
// refused, since raw leaves are not the target's input. refactor.PromoteFuzzSeed
// owns the "never corrupt user code" contract and splices literals only. On
// any failure the crasher file is left in place and ok is false; only once
// the splice has landed on disk is the file removed.
func promoteCrasher(overlay *gotestrunner.OverlayResult, target gotestrunner.FuzzTarget, suite, method, file string, typeNames refactor.TypeNames, env []string) (message string, ok bool) { //nolint:gocritic // hugeParam: stable API
	hashBase := filepath.Base(file)

	corpusArgs, perr := parseCorpusFile(file)
	if perr != nil {
		return fmt.Sprintf("promote: %s", perr), false
	}

	var spliceArgs []string
	if overlay.FuzzDecodedByFunc[target.Package][target.Func] {
		out, _, _ := rerunCrasher(overlay, target, hashBase, env)
		lit := extractDecodedInput(out)
		if lit == "" {
			leaves := len(overlay.FuzzParamsByFunc[target.Package][target.Func])
			return fmt.Sprintf("promote: %s/%s: skipped: %s decodes its input from %d lea%s, and the re-run echoed no decoded value; raw leaves are not its input",
				target.Func, hashBase, target.Func, leaves, leafSuffix(leaves)), false
		}
		spliceArgs = []string{lit}
	} else {
		spliceArgs = make([]string, len(corpusArgs))
		for i, a := range corpusArgs {
			spliceArgs[i] = a.spliceExpr()
		}
	}

	editedFile, line, err := refactor.PromoteFuzzSeed(target.Dir, suite, method, spliceArgs, typeNames)
	if err != nil {
		return fmt.Sprintf("promote: %s/%s: skipped: %s", target.Func, hashBase, err), false
	}

	if err := os.Remove(file); err != nil {
		return fmt.Sprintf("promote: %s/%s: warning: seed added but could not remove crasher file: %s", target.Func, hashBase, err), true
	}

	return fmt.Sprintf("promoted %s/%s -> f.Add(%s) in %s:%d", target.Func, hashBase, strings.Join(spliceArgs, ", "), displayPath(editedFile), line), true
}


func leafSuffix(n int) string {
	if n == 1 {
		return "f"
	}
	return "ves"
}
