package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/gotestaudit"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/pkgpattern"
)

var auditAllowed = flagSet("--record", "--base", "--base-record", "--head-record", "--fail-on", "--format", "--output",
	"--evidence", "--base-evidence", "--run", "--coverage", "--diff", "--agent", "--control",
	"--mutate", "--mutate-budget", "--mutate-strings", "--parallel")

// auditUsage is what an audit without a base is told.
const auditUsage = "audit compares the tree with a base: give --base-record=<file> with a record --record wrote, or --base=<ref> to record a commit of this checkout"

// runAudit records a tree, or compares a head record with a base one and
// says what the runs and the coverage given prove of the change. It runs a
// test only under --run.
func runAudit(inv Invocation) int { //nolint:gocritic // hugeParam: stable API
	own, rest, err := SplitArgs(inv.TagArgs(), auditAllowed)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	args, err := parseAuditArgs(own, rest)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	if args.record {
		rec, code := recordWorkingTree(args.patterns, args.loadFlags)
		if code == 2 && rec == nil {
			return 2
		}
		if werr := writeAuditOutput(args.output, func(w io.Writer) error { return gotestaudit.WriteJSON(w, rec) }); werr != nil {
			fmt.Fprintf(os.Stderr, "FAIL: %s\n", werr)
			return 2
		}
		return code
	}

	head, code := args.headRecord()
	if code != 0 {
		return code
	}
	var git *gitBase
	if args.baseRef != "" {
		if git, err = resolveBase(args.baseRef); err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
			return 2
		}
		if note := ancestryNote(git); note != "" {
			fmt.Fprintln(os.Stderr, note)
		}
	}
	base, err := args.baseRecord(head, git)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	doc := gotestaudit.Compare(base, head)
	if err := args.prove(&doc, base, head, git); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	verdict := gotestaudit.Verdict{ExitCode: gotestaudit.ExitCode(&doc, args.failOn), FailOn: args.failOn, Verbose: args.verbose}
	if args.stopped {
		verdict.ExitCode = 130 // as the stopped run exits
	}
	err = writeAuditOutput(args.output, func(w io.Writer) error {
		switch args.format {
		case "json":
			return gotestaudit.WriteJSON(w, &doc)
		case "md":
			gotestaudit.RenderMarkdown(w, &doc, verdict)
		default:
			gotestaudit.RenderText(w, &doc, verdict)
		}
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	return verdict.ExitCode
}

// auditArgs is one audit invocation, read and checked.
type auditArgs struct {
	record              bool
	baseRef, baseFile   string
	headFile            string
	format, output      string
	failOn              []string
	patterns, loadFlags []string
	patternsGiven       bool
	// evidence and baseEvidence are the streams given for each side.
	evidence, baseEvidence []string
	run, verbose           bool
	coverage, diff         string
	// control hands standard input to the run --run makes; stopped says a
	// stop ended that run before its end.
	control, stopped bool
	// mutate judges the changed lines' mutants within mutateBudget;
	// mutateStrings adds the string operator. parallel is --parallel's
	// method budget, 0 for the runner's own.
	mutate, mutateStrings bool
	mutateBudget          time.Duration
	parallel              int
	// failOnNamed are the classes --fail-on names as written.
	failOnNamed []string
}

func parseAuditArgs(own OwnFlags, rest []string) (auditArgs, error) {
	a := auditArgs{
		record:        own.Bool("--record"),
		baseRef:       own.Value("--base"),
		baseFile:      own.Value("--base-record"),
		headFile:      own.Value("--head-record"),
		output:        own.Value("--output"),
		evidence:      own.Values("--evidence"),
		baseEvidence:  own.Values("--base-evidence"),
		run:           own.Bool("--run"),
		coverage:      own.Value("--coverage"),
		diff:          own.Value("--diff"),
		control:       own.Given("--control"),
		mutate:        own.Bool("--mutate"),
		mutateStrings: own.Bool("--mutate-strings"),
		mutateBudget:  defaultMutateBudget,
	}
	if a.mutate {
		a.run = true
	}
	if own.Given("--mutate-budget") {
		d, err := time.ParseDuration(own.Value("--mutate-budget"))
		if err != nil || d <= 0 {
			return a, fmt.Errorf("invalid --mutate-budget %q: give a positive duration such as 3m", own.Value("--mutate-budget"))
		}
		a.mutateBudget = d
	}
	if own.Given("--parallel") {
		n, err := strconv.Atoi(own.Value("--parallel"))
		if err != nil || n < 1 {
			return a, fmt.Errorf("invalid --parallel %q: give a positive count", own.Value("--parallel"))
		}
		a.parallel = n
	}
	if _, err := parseControlFlag(own); err != nil {
		return a, err
	}
	switch f := own.ValueOr("--format", "text"); f {
	case "text", "json":
		a.format = f
	case "md", "markdown":
		a.format = "md"
	default:
		return a, fmt.Errorf("invalid --format %q: audit writes text, json or md", f)
	}
	if own.Given("--fail-on") {
		classes, err := gotestaudit.ParseClasses(own.Value("--fail-on"))
		if err != nil {
			return a, fmt.Errorf("--fail-on: %w", err)
		}
		a.failOn = classes
		a.failOnNamed = strings.Split(own.Value("--fail-on"), ",")
	}
	rest, a.verbose = cutVerbose(rest)
	classified := gotestrunner.ClassifyGoTestArgs(rest)
	if len(classified.RunFlags) > 0 {
		return a, fmt.Errorf("audit takes no flag of a run; %s is one", classified.RunFlags[0])
	}
	a.loadFlags = gotestrunner.StripNonLoadFlags(classified.BuildFlags)
	a.patternsGiven = len(classified.PkgPatterns) > 0
	a.patterns = []string{"./..."}
	if a.patternsGiven {
		a.patterns = pkgpattern.Clean(classified.PkgPatterns)
	}
	return a, a.check(own)
}

// cutVerbose takes go test's -v out of args: in audit it lists every row
// the text format groups.
func cutVerbose(args []string) (rest []string, verbose bool) {
	for _, arg := range args {
		switch arg {
		case "-v", "-v=true", "--v":
			verbose = true
		case "-v=false":
			verbose = false
		default:
			rest = append(rest, arg)
		}
	}
	return rest, verbose
}

// proofFlags are the flags that say what runs and coverage prove.
var proofFlags = []string{"--evidence", "--base-evidence", "--run", "--coverage", "--diff", "--mutate"}

// checkMutate refuses the mutation flags where they cannot apply.
func (a *auditArgs) checkMutate(own OwnFlags) error {
	for _, f := range []string{"--mutate-budget", "--mutate-strings"} {
		if own.Given(f) && !a.mutate {
			return fmt.Errorf("%s goes with --mutate", f)
		}
	}
	if own.Given("--parallel") && !a.run {
		return errors.New("--parallel bounds the run --run or --mutate makes; give it with one")
	}
	for _, c := range a.failOnNamed {
		if strings.TrimSpace(c) == gotestaudit.ClassSurvived && !a.mutate {
			return errors.New("--fail-on=survived judges mutants: give --mutate, with --base or --diff")
		}
	}
	if a.mutate && !a.record && a.diff == "" && a.baseRef == "" {
		return errors.New("--mutate mutates the changed lines: give --base=<ref> or --diff=<file>")
	}
	return nil
}

// check refuses flags that do not go together.
func (a *auditArgs) check(own OwnFlags) error {
	if a.control {
		switch {
		case !a.run:
			return errors.New("--control=stdin stops the run --run makes; give it with --run")
		case a.diff == "-" || slices.Contains(a.evidence, "-") || slices.Contains(a.baseEvidence, "-"):
			return errors.New("--control=stdin and a stream or diff from - both read standard input; save it to a file")
		}
	}
	if err := a.checkMutate(own); err != nil {
		return err
	}
	if a.record {
		for _, f := range append([]string{"--base", "--base-record", "--head-record", "--fail-on"}, proofFlags...) {
			if own.Given(f) {
				return fmt.Errorf("--record writes the record of the tree; it does not take %s", f)
			}
		}
		if own.Given("--format") && a.format != "json" {
			return errors.New("--record writes JSON; it takes no other --format")
		}
		return nil
	}
	switch {
	case a.baseRef != "" && a.baseFile != "":
		return errors.New("--base and --base-record both name the base; give one")
	case a.baseRef == "" && a.baseFile == "":
		return errors.New(auditUsage)
	case a.headFile != "" && a.patternsGiven:
		return errors.New("--head-record names the head; the packages to record go with the working tree, not with it")
	case a.run && a.headFile != "":
		return errors.New("--run runs the working tree; it does not go with --head-record, which names another head")
	case a.diff != "" && a.baseRef != "":
		return errors.New("--base and --diff both give the changed lines; give one")
	case a.diff != "" && a.coverage == "" && !a.run:
		return errors.New("--diff gives the changed lines the unproven ones are read from; give --coverage=<profile> or --run with it")
	case a.coverage != "" && a.diff == "" && a.baseRef == "":
		return errors.New("--coverage needs the lines that changed: give --base=<ref> or --diff=<file>")
	}
	for _, c := range a.failOn {
		switch {
		case c == gotestaudit.ClassUnproven && !a.unproven():
			return errors.New("--fail-on=unproven judges a coverage profile: give --coverage or --run, with --base or --diff")
		case gotestaudit.IsProof(c) && c != gotestaudit.ClassUnproven && len(a.evidence) == 0 && !a.run:
			return fmt.Errorf("--fail-on=%s judges runs: give --evidence=<stream> or --run", c)
		}
	}
	return nil
}

// unproven reports whether the unproven lines are asked for: a profile, or
// a run, and the lines that changed.
func (a *auditArgs) unproven() bool {
	return (a.coverage != "" || a.run) && (a.diff != "" || a.baseRef != "")
}

// headRecord is the head: the record given, else the working tree's.
func (a *auditArgs) headRecord() (*gotestaudit.Record, int) {
	if a.headFile != "" {
		rec, err := readRecordFile(a.headFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: --head-record: %s\n", err)
			return nil, 2
		}
		return rec, 0
	}
	rec, _ := recordWorkingTree(a.patterns, a.loadFlags)
	if rec == nil {
		return nil, 2
	}
	return rec, 0
}

// baseRecord is the base: the record given, else the record of the commit
// --base names.
func (a *auditArgs) baseRecord(head *gotestaudit.Record, git *gitBase) (*gotestaudit.Record, error) {
	if a.baseFile != "" {
		rec, err := readRecordFile(a.baseFile)
		if err != nil {
			return nil, fmt.Errorf("--base-record: %w", err)
		}
		return rec, nil
	}
	return recordRef(git, a.recordedPatterns(head), a.loadFlags)
}

// recordedPatterns are the packages the head was recorded with.
func (a *auditArgs) recordedPatterns(head *gotestaudit.Record) []string {
	if a.headFile != "" {
		return head.Patterns
	}
	return a.patterns
}

// recordWorkingTree records the tree in the current directory. Its code is
// 2 when a package could not be read, the record then still written; a nil
// record is a load that failed, said on stderr.
func recordWorkingTree(patterns, loadFlags []string) (*gotestaudit.Record, int) {
	prov := gotestaudit.Provenance{}
	if cwd, err := os.Getwd(); err == nil {
		if p := readProvenance(cwd); p.Commit != "" {
			dirty := p.Dirty
			prov = gotestaudit.Provenance{Commit: p.Commit, Dirty: &dirty}
			if p.Branch != "HEAD" {
				prov.Branch = p.Branch
			}
		}
	}
	rec, unmatched, err := gotestaudit.Read(context.Background(), &gotestaudit.Options{Patterns: patterns, LoadFlags: loadFlags, Provenance: prov})
	for _, p := range unmatched {
		fmt.Fprintf(os.Stderr, "warning: %q matched no packages\n", p)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return nil, 2
	}
	code := 0
	for _, p := range rec.Packages {
		if len(p.Unreadable) > 0 {
			fmt.Fprintf(os.Stderr, "audit: %s cannot be read: %s\n", p.Path, p.Unreadable[0])
			code = 2
		}
	}
	for _, u := range rec.Unread {
		fmt.Fprintf(os.Stderr, "audit: %s\n", u)
		code = 2
	}
	return &rec, code
}

// readRecordFile reads a record --record wrote.
func readRecordFile(path string) (*gotestaudit.Record, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: the record the user named
	if err != nil {
		return nil, err
	}
	var rec gotestaudit.Record
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, fmt.Errorf("%s is no audit record: %w", path, err)
	}
	switch {
	case rec.Schema == 0:
		return nil, fmt.Errorf("%s is no audit record: it has no schema", path)
	case rec.Schema > gotestaudit.Schema:
		return nil, fmt.Errorf("%s has schema %d, newer than this gotest's %d; compare it with the gotest that wrote it", path, rec.Schema, gotestaudit.Schema)
	}
	return &rec, nil
}

// writeAuditOutput writes to --output's file, else stdout.
func writeAuditOutput(output string, write func(io.Writer) error) error {
	if output == "" {
		return write(os.Stdout)
	}
	f, err := os.Create(output) //nolint:gosec // G304: the file the user named
	if err != nil {
		return fmt.Errorf("creating output file: %w", err)
	}
	if err := write(f); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func printAuditHelp() {
	fmt.Print(`gotest audit — what a change did to the test estate, and what runs proved of it

Usage:
  gotest audit --record [--output=<file>] [packages]
  gotest audit --base=<ref> [flags] [packages]
  gotest audit --base-record=<file> [--head-record=<file>] [flags] [packages]

--record writes the record of a tree, read from source: every suite, test,
When, It and row it declares, with its file and lines, a hash of its body
and the assertions it writes; each suite's budgets and guard; every
suppression and snapshot file. An unchanged tree records the same bytes.

Without --record, audit compares a head record with a base one: the
behaviors added, removed, reworded, moved and changed, and the findings that
mean the estate got weaker. The head is the working tree, recorded on the
spot, unless --head-record names a record. The base is a record, or the
commit --base names, exported by git into gotest's cache, kept there for
the next audit of the same commit, and recorded from the same directory.

Evidence: --evidence names a saved -json stream of a run of the head
(--events-file writes one), --run runs the packages of the changed
behaviors itself. Each behavior the comparison names gets its verdict, its
run-time assertions and the stream it came from, current when that stream
ran the head's commit with neither tree dirty, else stale. --coverage and
the changed lines (git diff of --base and the files on disk, or --diff)
list the changed production lines no test executed. --mutate asks, of each
changed line a test executed, whether a test notices it made wrong.

Packages default to ./... .

Flags:
  --record              Write the record of the tree as JSON, compare nothing
  --base=<ref>          Record the commit ref names as the base
                        (the merge base: --base=$(git merge-base origin/main HEAD))
  --base-record=<file>  The base, a record --record wrote
  --head-record=<file>  The head, a record --record wrote
  --evidence=<stream>   A -json stream of a run of the head; repeatable
  --base-evidence=<stream>
                        A -json stream of a run of the base, for the run-time
                        assertions of an assertions finding; repeatable
  --run                 Run the packages of the changed behaviors, with
                        coverage of the changed production packages, as
                        current evidence
  --mutate              Run as --run does, then judge the mutants of the
                        changed lines by the suites that executed each:
                        killed, survived, timeout, or not judged once the
                        budget is spent. //gotest:mutate-ignore <reason>
                        keeps a line or a function from mutation
  --mutate-budget=<d>   The time the mutants may take (default 3m)
  --mutate-strings      Mutate string literals too
  --parallel=<n>        The tests the runs make at once (default: the
                        runner's own)
  --coverage=<file>     A coverage profile of the head, for the unproven lines
  --diff=<file|->       The changed lines as a unified diff, without --base
  --fail-on=<classes>   Exit 1 when the document lists a finding of one of the
                        comma-separated classes: removed, excluded, focus,
                        guard, budget, suppression, snapshot, assertions,
                        asserts-by, changed; all is every class but changed.
                        Proof classes: failing, not-run, vacuous, stale,
                        unproven, survived; proof is the six
  --format=<fmt>        text (default), json or md
  --output=<file>       Write to a file instead of stdout
  --control=stdin       Hand stop and force lines on stdin to the run --run
                        makes; a stopped run exits 130, its document written
  -v                    List every row the text format groups
  --agent=<bool>        Taken as a run takes it; audit prints the same either way

Exit codes: 0 compared (whatever it found, without --fail-on); 1 --fail-on
found its class; 2 a flag, a record or a stream is wrong, git or the ref is
missing, or a package could not be read on one side, which then is not
compared; 130 the run --run made was stopped.

Examples:
  gotest audit --base=origin/main
  gotest audit --base=origin/main --run --fail-on=all,proof
  gotest audit --base=origin/main --mutate --mutate-budget=5m --fail-on=survived
  gotest audit --base=origin/main --evidence=run.json --coverage=cover.out
  gotest audit --record --output=base.json ./...
  gotest audit --base-record=base.json --fail-on=removed,focus,excluded --format=md
`)
}
