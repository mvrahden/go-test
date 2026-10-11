package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/mvrahden/go-test/internal/config"
	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestspec"
	"github.com/mvrahden/go-test/internal/lint"
	"github.com/mvrahden/go-test/internal/unidiff"
	"golang.org/x/tools/go/analysis/singlechecker"
)

func runLint(inv Invocation) int { //nolint:gocritic // hugeParam: stable API
	github := lintGitHubArmed(inv.Args)
	args := stripFlag(inv.Args, "--github")

	flagArgs, err := lintSkipFlags(args, inv.Config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	args = append(flagArgs, args...)

	// Lint owns the analyzer's flags, -fix, -diff, -json, -test and -c. A
	// flag only the go/analysis driver implements (-debug, -V, …) goes to
	// that driver, which keeps its exact semantics but cannot emit
	// annotations.
	if code, ok := runLintInProcess(os.Stdout, os.Stderr, "", args, github); ok {
		return code
	}

	a, _ := parseLintArgs(args)
	// The driver ignores -tags; it reads GOFLAGS, as go list does.
	if a.tags != "" {
		_ = os.Setenv("GOFLAGS", strings.TrimSpace(os.Getenv("GOFLAGS")+" -tags="+a.tags))
	}
	// The driver skips uncompilable packages and still exits 0, so a broken
	// tree lints "clean". Prove the targets compile first.
	if a.loads() {
		if err := lint.PreflightLoad("", a.patterns); err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: %v\n", err)
			return 1
		}
		for _, p := range gotestgen.UnmatchedPatterns("", a.patterns, nil) {
			fmt.Fprintf(os.Stderr, "warning: %q matched no packages\n", p)
		}
	}
	runLintDriver(a.argv())
	fmt.Fprintln(os.Stderr, "lint: unexpected return from singlechecker.Main")
	return 2
}

// runLintDriver hands args to the go/analysis driver, which exits.
func runLintDriver(args []string) {
	os.Args = append([]string{"gotest lint"}, args...)
	singlechecker.Main(lint.Analyzer)
}

// lintArgs is lint's argv, read the way the go/analysis driver's flag set
// reads it, except that flags may also follow the patterns.
type lintArgs struct {
	analyzer []string // -skip-<rule>, -disable-nolint
	driver   []string // flags only the go/analysis driver implements
	patterns []string
	fix      bool
	diff     bool
	json     bool
	tests    bool   // -test
	context  int    // -c; negative prints no source lines
	tags     string // -tags, as for go build
	noLoad   bool   // -flags or -V: the driver answers without loading
	unknown  bool   // a flag nobody defines: the driver reports it
}

// lintValueFlags take a value, as -name=value or as the next argument.
var lintValueFlags = map[string]bool{
	"c": true, "tags": true, "debug": true, "cpuprofile": true, "memprofile": true, "trace": true,
}

func parseLintArgs(args []string) (lintArgs, error) {
	a := lintArgs{tests: true, context: -1}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			a.patterns = append(a.patterns, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			a.patterns = append(a.patterns, arg)
			continue
		}
		name, value := splitBoolFlag(arg)
		if lintValueFlags[name] && !strings.Contains(arg, "=") {
			if i+1 == len(args) {
				return a, fmt.Errorf("flag needs an argument: -%s", name)
			}
			i++
			value = args[i]
		}
		var err error
		switch {
		case lintAnalyzerFlag(name):
			a.analyzer = append(a.analyzer, "-"+name+"="+value)
		case name == "fix":
			a.fix, err = strconv.ParseBool(value)
		case name == "diff":
			a.diff, err = strconv.ParseBool(value)
		case name == "json":
			a.json, err = strconv.ParseBool(value)
		case name == "test":
			a.tests, err = strconv.ParseBool(value)
		case name == "c":
			a.context, err = strconv.Atoi(value)
		case name == "tags":
			a.tags = value
		case name == "source" || name == "all":
			// No effect in the driver either (deprecated).
		case name == "flags" || name == "V":
			a.noLoad = true
			a.driver = append(a.driver, arg)
		case lintValueFlags[name] || name == "v":
			a.driver = append(a.driver, "-"+name+"="+value)
		default:
			a.unknown = true
			a.driver = append(a.driver, arg)
		}
		if err != nil {
			return a, fmt.Errorf("invalid value %q for flag -%s", value, name)
		}
	}
	if len(a.patterns) == 0 {
		a.patterns = []string{"./..."}
	}
	// -diff previews -fix, so it implies it.
	a.fix = a.fix || a.diff
	return a, nil
}

// inProcess reports whether lint can run without the go/analysis driver.
func (a *lintArgs) inProcess() bool {
	return len(a.driver) == 0
}

// loads reports whether the driver will load the targets.
func (a *lintArgs) loads() bool {
	return !a.noLoad && !a.unknown
}

// buildFlags returns the go build flags the args carry.
func (a *lintArgs) buildFlags() []string {
	if a.tags == "" {
		return nil
	}
	return []string{"-tags=" + a.tags}
}

// argv returns the args for the go/analysis driver, flags first.
func (a *lintArgs) argv() []string {
	out := slices.Clone(a.driver)
	out = append(out, a.analyzer...)
	if a.fix {
		out = append(out, "-fix")
	}
	if a.json {
		out = append(out, "-json")
	}
	if !a.tests {
		out = append(out, "-test=false")
	}
	if a.context >= 0 {
		out = append(out, "-c="+strconv.Itoa(a.context))
	}
	return append(out, a.patterns...)
}

// runLintInProcess runs the analyzer over the targets and reports what it
// found: JSON on stdout with -json, GitHub annotations on stdout with
// github, plain text on stderr otherwise. With -fix it first applies the
// fixes and reports only what is left; with -diff it prints what -fix would
// write instead, writes nothing, and reports what -fix would leave. ok=false
// means args carry a flag only the go/analysis driver implements; nothing
// has run yet.
//
// dir is the module directory to load from; "" means the current directory.
// Annotation paths are relative to dir, which inside a workflow is the
// repository root — exactly what GitHub resolves annotations against.
func runLintInProcess(stdout, stderr io.Writer, dir string, args []string, github bool) (code int, ok bool) {
	a, err := parseLintArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "FAIL: %s\n", err)
		return 2, true
	}
	if a.fix && a.loads() && len(a.driver) > 0 {
		fmt.Fprintf(stderr, "FAIL: %s is a go/analysis driver flag; -fix and -diff do not take it\n", a.driver[0])
		return 2, true
	}
	if !a.inProcess() {
		return 0, false
	}
	if a.diff && a.json {
		fmt.Fprintln(stderr, "FAIL: -diff and -json both print on stdout; use one")
		return 2, true
	}
	for _, f := range a.analyzer {
		name, value := splitBoolFlag(f)
		if err := lint.Analyzer.Flags.Set(name, value); err != nil {
			fmt.Fprintf(stderr, "FAIL: %s\n", err)
			return 2, true
		}
	}

	targets := lint.Targets{Dir: dir, Patterns: a.patterns, Tests: a.tests, BuildFlags: a.buildFlags()}
	ctx := context.Background()
	var written []string // the files -fix wrote, for a second signal to name
	if a.fix {
		// The load's temporary files, the go command's included, live in
		// one directory, removed however the run ends.
		tmp, err := os.MkdirTemp(os.Getenv("GOTMPDIR"), "gotest-lint-")
		if err != nil {
			fmt.Fprintf(stderr, "FAIL: %v\n", err)
			return 1, true
		}
		defer func() { _ = os.RemoveAll(tmp) }()
		targets.TempDir = tmp
		// Nothing is written until every pass is done, so a stop before
		// then leaves every file as it was; writing holds off a second
		// signal until it is done.
		var stop context.CancelFunc
		ctx, stop = shutdownContextThen(func() {
			_ = os.RemoveAll(tmp)
			if len(written) == 0 {
				fmt.Fprintln(stderr, "lint: interrupted again; nothing was written")
			} else {
				fmt.Fprintf(stderr, "lint: interrupted again; wrote fixes to %d file(s)\n", len(written))
			}
		})
		defer stop()
	}
	interrupted := func() bool {
		if ctx.Err() == nil {
			return false
		}
		fmt.Fprintln(stderr, "lint: interrupted; nothing was written")
		return true
	}

	// This load also proves the targets compile before anything is fixed.
	res, err := lint.Analyze(ctx, targets)
	if interrupted() {
		return 130, true
	}
	if err != nil {
		fmt.Fprintf(stderr, "FAIL: %v\n", err)
		return 1, true
	}
	for _, p := range gotestgen.UnmatchedPatterns(dir, a.patterns, targets.BuildFlags) {
		fmt.Fprintf(stderr, "warning: %q matched no packages\n", p)
	}
	source := os.ReadFile
	if a.fix && len(res.FixFiles) > 0 {
		fixed, changes, err := lint.Fix(ctx, targets, res)
		if interrupted() {
			return 130, true
		}
		if err != nil {
			fmt.Fprintf(stderr, "FAIL: %v\n", err)
			return 1, true
		}
		if a.diff {
			printLintDiff(stdout, stderr, dir, changes)
			source = lintChangedSource(changes)
		} else if code := writeLintFixes(ctx, stderr, changes, &written); code != 0 {
			return code, true
		}
		res = fixed
	}

	switch {
	case a.json:
		// As with the driver, -json exits 0 with findings.
		if err := res.PrintJSON(stdout); err != nil {
			fmt.Fprintf(stderr, "FAIL: %v\n", err)
			return 1, true
		}
		return 0, true
	case github && !a.diff:
		return reportLintGitHub(stdout, dir, res.Findings), true
	default:
		return reportLintPlain(stderr, res.Findings, a.context, source), true
	}
}

// writeLintFixes writes every fixed file or none: each goes to a temporary
// file beside the one it replaces, and only once all are staged are they
// renamed over the originals. A second signal waits until it is done. It
// returns 0 once every file is written; otherwise it says on stderr what
// was written and returns the exit code: 130 when a signal came first, 1
// on a failure.
func writeLintFixes(ctx context.Context, stderr io.Writer, changes []lint.Change, written *[]string) int {
	forcedExit.Lock()
	defer forcedExit.Unlock()
	if ctx.Err() != nil {
		fmt.Fprintln(stderr, "lint: interrupted; nothing was written")
		return 130
	}

	type staged struct{ file, target, tmp string }
	var all []staged
	discard := func(rest []staged) {
		for _, st := range rest {
			_ = os.Remove(st.tmp)
		}
	}
	for _, c := range changes {
		target, tmp, err := stageLintFix(c)
		if err != nil {
			discard(all)
			fmt.Fprintf(stderr, "FAIL: writing fixes: %v; nothing was written\n", err)
			return 1
		}
		all = append(all, staged{c.File, target, tmp})
	}
	for i, st := range all {
		if err := os.Rename(st.tmp, st.target); err != nil {
			discard(all[i:])
			fmt.Fprintf(stderr, "FAIL: writing fixes: %v\n", err)
			if len(*written) > 0 {
				fmt.Fprintf(stderr, "lint: wrote fixes to:\n\t%s\n", strings.Join(*written, "\n\t"))
			}
			rest := make([]string, 0, len(all)-i)
			for _, st := range all[i:] {
				rest = append(rest, st.file)
			}
			fmt.Fprintf(stderr, "lint: did not write:\n\t%s\n", strings.Join(rest, "\n\t"))
			return 1
		}
		*written = append(*written, st.file)
	}
	return 0
}

// stageLintFix writes c's fixed contents to a temporary file beside the
// file it replaces, with that file's permissions, and returns both. The
// file a symlink names is the one replaced, so the link stays.
func stageLintFix(c lint.Change) (target, tmp string, err error) {
	if target, err = filepath.EvalSymlinks(c.File); err != nil {
		return "", "", err
	}
	info, err := os.Stat(target)
	if err != nil {
		return "", "", err
	}
	// A rename would replace a file its permissions keep from being written.
	f, err := os.OpenFile(target, os.O_WRONLY, 0)
	if err != nil {
		return "", "", err
	}
	_ = f.Close()
	if disk, err := os.ReadFile(target); err != nil || !bytes.Equal(disk, c.Before) {
		return "", "", fmt.Errorf("%s changed since lint read it", c.File)
	}

	t, err := os.CreateTemp(filepath.Dir(target), "."+filepath.Base(target)+".gotest-*")
	if err != nil {
		return "", "", err
	}
	_, err = t.Write(c.After)
	if err == nil {
		err = t.Chmod(info.Mode().Perm())
	}
	if cerr := t.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(t.Name())
		return "", "", err
	}
	return target, t.Name(), nil
}

// printLintDiff prints changes as one unified diff. Paths are relative to
// dir, with a/ and b/ prefixes: git apply resolves the paths of a patch
// without "diff --git" headers against the directory it runs in, inside a
// repository or not. A file outside dir gets a ../ path, which git apply
// only takes with --unsafe-paths; stderr says so.
func printLintDiff(stdout, stderr io.Writer, dir string, changes []lint.Change) {
	base := dir
	if base == "" {
		base, _ = os.Getwd()
	}
	outside := false
	for _, c := range changes {
		oldName, newName := filepath.ToSlash(c.File), filepath.ToSlash(c.File)
		if rel, err := filepath.Rel(base, c.File); err == nil {
			rel = filepath.ToSlash(rel)
			oldName, newName = "a/"+rel, "b/"+rel
			outside = outside || strings.HasPrefix(rel, "../")
		}
		fmt.Fprint(stdout, unidiff.Diff(oldName, newName, c.Before, c.After))
	}
	if outside {
		fmt.Fprintln(stderr, "lint: the diff names files outside the current directory; apply it with git apply --unsafe-paths")
	}
}

// lintChangedSource reads a file as -fix would leave it.
func lintChangedSource(changes []lint.Change) func(string) ([]byte, error) {
	return func(file string) ([]byte, error) {
		for _, c := range changes {
			if c.File == file {
				return c.After, nil
			}
		}
		return os.ReadFile(file)
	}
}

// lintGitHubArmed reports whether lint should render GitHub annotations:
// explicitly via --github, or implicitly inside a GitHub Actions runner,
// mirroring the summary subcommand's arming.
func lintGitHubArmed(args []string) bool {
	return hasFlag(args, "--github") || os.Getenv("GITHUB_ACTIONS") == "true"
}

// stripFlag returns args without any occurrence of name or name=value.
func stripFlag(args []string, name string) []string {
	var out []string
	for _, a := range args {
		if a == name || strings.HasPrefix(a, name+"=") {
			continue
		}
		out = append(out, a)
	}
	return out
}

// reportLintGitHub renders the findings two ways: ::error workflow commands
// on stdout and a markdown table appended to $GITHUB_STEP_SUMMARY. Findings
// deliberately stay off stderr — setup-go registers a problem matcher for
// file:line:col text, and a stderr mirror would surface every finding as a
// second, job-titled annotation.
func reportLintGitHub(stdout io.Writer, dir string, findings []lint.Finding) int {
	base := dir
	if base == "" {
		base, _ = os.Getwd()
	}

	annotations := make([]gotestspec.Annotation, 0, len(findings))
	for _, f := range findings {
		annotations = append(annotations, gotestspec.Annotation{
			File:    relPath(base, f.File),
			Line:    f.Line,
			Col:     f.Col,
			Title:   f.Rule,
			Message: f.Message,
		})
	}
	gotestspec.WriteGitHubAnnotations(stdout, annotations)

	if len(annotations) == 0 {
		return 0
	}
	appendLintStepSummary(annotations)
	return 3
}

// reportLintPlain prints the findings on stderr, one "file:line:col:
// message" line each, the way the go/analysis driver does; a non-negative
// context adds the offending lines with that many lines around them.
func reportLintPlain(stderr io.Writer, findings []lint.Finding, context int, source func(string) ([]byte, error)) int {
	for _, f := range findings {
		fmt.Fprintf(stderr, "%s:%d:%d: %s\n", f.File, f.Line, f.Col, f.Message)
		if context < 0 {
			continue
		}
		data, _ := source(f.File)
		lines := strings.Split(string(data), "\n")
		for i := max(f.Line-context, 1); i <= min(f.EndLine+context, len(lines)); i++ {
			fmt.Fprintf(stderr, "%d\t%s\n", i, lines[i-1])
		}
	}
	if len(findings) == 0 {
		return 0
	}
	return 3
}

// lintAnalyzerFlag reports whether name (without dashes) is one of the
// analyzer's own flags.
func lintAnalyzerFlag(name string) bool {
	if name == "disable-nolint" {
		return true
	}
	rule, isSkip := strings.CutPrefix(name, "skip-")
	return isSkip && lint.SkippableRules[lint.Rule(rule)]
}

// splitBoolFlag splits "-name[=value]" into (name, value), defaulting the
// value to "true" the way the flag package treats bare boolean flags.
func splitBoolFlag(arg string) (name, value string) {
	name = strings.TrimLeft(arg, "-")
	value = "true"
	if eq := strings.IndexByte(name, '='); eq >= 0 {
		name, value = name[:eq], name[eq+1:]
	}
	return name, value
}

func relPath(base, file string) string {
	rel, err := filepath.Rel(base, file)
	if err != nil || strings.HasPrefix(rel, "..") {
		return file
	}
	return filepath.ToSlash(rel)
}

// appendLintStepSummary appends the findings table to the workflow's step
// summary — the complete record when GitHub caps rendered annotations.
func appendLintStepSummary(annotations []gotestspec.Annotation) {
	path := os.Getenv("GITHUB_STEP_SUMMARY")
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	fmt.Fprintf(f, "### gotest lint — %d finding(s)\n\n", len(annotations))
	fmt.Fprintln(f, "| Rule | Location | Message |")
	fmt.Fprintln(f, "| --- | --- | --- |")
	for _, a := range annotations {
		msg := strings.ReplaceAll(a.Message, "|", "\\|")
		fmt.Fprintf(f, "| %s | %s:%d | %s |\n", a.Title, a.File, a.Line, msg)
	}
	fmt.Fprintln(f)
}

// lintSkipFlags returns analyzer flags derived from the config's lint.skip
// list, omitting any rules that are already set via CLI args.
func lintSkipFlags(args []string, cfg config.ProjectConfig) ([]string, error) { //nolint:gocritic // hugeParam: stable API
	var flags []string
	for _, rule := range cfg.Lint.Skip {
		if !lint.SkippableRules[lint.Rule(rule)] {
			if lint.Known(lint.Rule(rule)) {
				return nil, fmt.Errorf("integrity lint rule in %s: %q can only be suppressed per line with //nolint", config.FileName, rule)
			}
			return nil, fmt.Errorf("unknown lint rule in %s: %q", config.FileName, rule)
		}
		flag := "-skip-" + rule
		if !slices.Contains(args, flag) && !slices.Contains(args, flag+"=true") && !slices.Contains(args, flag+"=false") {
			flags = append(flags, flag)
		}
	}
	return flags, nil
}
