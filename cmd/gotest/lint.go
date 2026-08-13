package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mvrahden/go-test/internal/config"
	"github.com/mvrahden/go-test/internal/gotestspec"
	"github.com/mvrahden/go-test/internal/lint"
	"golang.org/x/tools/go/analysis/singlechecker"
)

func runLint(inv Invocation) int { //nolint:gocritic // hugeParam: stable API
	github := lintGitHubArmed(inv.Args)
	args := stripFlag(inv.Args, "--github")
	if len(args) == 0 {
		args = []string{"./..."}
	}

	flagArgs, err := lintSkipFlags(args, inv.Config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	if github {
		if code, ok := runLintGitHub(os.Stdout, os.Stderr, "", append(flagArgs, args...)); ok {
			return code
		}
		// Driver flags this mode does not own (-fix, -json, …): fall through
		// to the singlechecker, which keeps their exact semantics but cannot
		// emit annotations.
	}

	// The singlechecker driver skips uncompilable packages and still exits 0,
	// so a broken tree lints "clean". Prove the targets compile first.
	if patterns := lint.PreflightPatterns(args); len(patterns) > 0 {
		if err := lint.PreflightLoad("", patterns); err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: %v\n", err)
			return 1
		}
	}

	os.Args = append([]string{"gotest lint"}, append(flagArgs, args...)...)
	singlechecker.Main(lint.Analyzer)

	fmt.Fprintln(os.Stderr, "lint: unexpected return from singlechecker.Main")
	return 2
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

// runLintGitHub runs the analyzer programmatically and renders the findings
// three ways: plain text on stderr (matching the singlechecker driver),
// ::error workflow commands on stdout, and a markdown table appended to
// $GITHUB_STEP_SUMMARY. It owns only the analyzer's own flags; ok=false
// means an unrecognized driver flag was present and the caller must fall
// back to the singlechecker.
//
// dir is the module directory to load from; "" means the current directory.
// Annotation paths are relative to dir, which inside a workflow is the
// repository root — exactly what GitHub resolves annotations against.
func runLintGitHub(stdout, stderr io.Writer, dir string, args []string) (code int, ok bool) {
	var patterns []string
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			patterns = append(patterns, a)
			continue
		}
		name, value := splitBoolFlag(a)
		if !lintAnalyzerFlag(name) {
			return 0, false
		}
		if err := lint.Analyzer.Flags.Set(name, value); err != nil {
			fmt.Fprintf(stderr, "FAIL: %s\n", err)
			return 2, true
		}
	}

	findings, err := lint.Run(dir, patterns)
	if err != nil {
		fmt.Fprintf(stderr, "FAIL: %v\n", err)
		return 1, true
	}

	base := dir
	if base == "" {
		base, _ = os.Getwd()
	}

	annotations := make([]gotestspec.Annotation, 0, len(findings))
	for _, f := range findings {
		file := relPath(base, f.File)
		fmt.Fprintf(stderr, "%s:%d:%d: %s\n", file, f.Line, f.Col, f.Message)
		annotations = append(annotations, gotestspec.Annotation{
			File:    file,
			Line:    f.Line,
			Col:     f.Col,
			Title:   f.Rule,
			Message: f.Message,
		})
	}
	gotestspec.WriteGitHubAnnotations(stdout, annotations)

	if len(annotations) == 0 {
		return 0, true
	}
	appendLintStepSummary(annotations)
	return 3, true
}

// lintAnalyzerFlag reports whether name (without dashes) is one of the
// analyzer's own flags, the only ones the GitHub mode applies itself.
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
