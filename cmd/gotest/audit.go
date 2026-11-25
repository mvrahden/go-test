package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/mvrahden/go-test/internal/gotestaudit"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/pkgpattern"
)

var auditAllowed = flagSet("--record", "--base", "--base-record", "--head-record", "--fail-on", "--format", "--output")

// auditUsage is what an audit without a base is told.
const auditUsage = "audit compares the tree with a base: give --base-record=<file> with a record --record wrote, or --base=<ref> to record a commit of this checkout"

// runAudit records a tree, or compares a head record with a base one. It runs
// no test.
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
	base, err := args.baseRecord(head)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	doc := gotestaudit.Compare(base, head)
	verdict := gotestaudit.Verdict{ExitCode: gotestaudit.ExitCode(&doc, args.failOn), FailOn: args.failOn}
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
}

func parseAuditArgs(own OwnFlags, rest []string) (auditArgs, error) {
	a := auditArgs{
		record:   own.Bool("--record"),
		baseRef:  own.Value("--base"),
		baseFile: own.Value("--base-record"),
		headFile: own.Value("--head-record"),
		output:   own.Value("--output"),
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
	}
	classified := gotestrunner.ClassifyGoTestArgs(rest)
	if len(classified.RunFlags) > 0 {
		return a, fmt.Errorf("audit runs no test; %s is a flag of a run", classified.RunFlags[0])
	}
	a.loadFlags = gotestrunner.StripNonLoadFlags(classified.BuildFlags)
	a.patternsGiven = len(classified.PkgPatterns) > 0
	a.patterns = []string{"./..."}
	if a.patternsGiven {
		a.patterns = pkgpattern.Clean(classified.PkgPatterns)
	}
	return a, a.check(own)
}

// check refuses flags that do not go together.
func (a *auditArgs) check(own OwnFlags) error {
	if a.record {
		for _, f := range []string{"--base", "--base-record", "--head-record", "--fail-on"} {
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
	}
	return nil
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
func (a *auditArgs) baseRecord(head *gotestaudit.Record) (*gotestaudit.Record, error) {
	if a.baseFile != "" {
		rec, err := readRecordFile(a.baseFile)
		if err != nil {
			return nil, fmt.Errorf("--base-record: %w", err)
		}
		return rec, nil
	}
	patterns := a.patterns
	if a.headFile != "" {
		patterns = head.Patterns
	}
	return recordRef(a.baseRef, patterns, a.loadFlags)
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
	fmt.Print(`gotest audit — what a change did to the test estate, read from source

Usage:
  gotest audit --record [--output=<file>] [packages]
  gotest audit --base=<ref> [flags] [packages]
  gotest audit --base-record=<file> [--head-record=<file>] [flags] [packages]

Runs no test. --record writes the record of a tree: every suite, test, When,
It and row its source declares, with its file and lines, a hash of its body
and the assertions it writes; each suite's budgets and guard; every
suppression and snapshot file. An unchanged tree records the same bytes.

Without --record, audit compares a head record with a base one: the
behaviors added, removed, reworded, moved and changed, and the findings that
mean the estate got weaker. The head is the working tree, recorded on the
spot, unless --head-record names a record. The base is a record, or the
commit --base names, exported by git outside the checkout and recorded from
the same directory.

Packages default to ./... .

Flags:
  --record              Write the record of the tree as JSON, compare nothing
  --base=<ref>          Record the commit ref names as the base
                        (the merge base: --base=$(git merge-base origin/main HEAD))
  --base-record=<file>  The base, a record --record wrote
  --head-record=<file>  The head, a record --record wrote
  --fail-on=<classes>   Exit 1 when the document lists a finding of one of the
                        comma-separated classes: removed, excluded, focus,
                        guard, budget, suppression, snapshot, assertions,
                        asserts-by, changed; all is every class but changed
  --format=<fmt>        text (default), json or md
  --output=<file>       Write to a file instead of stdout

Exit codes: 0 compared (whatever it found, without --fail-on); 1 --fail-on
found its class; 2 a flag or a record is wrong, git or the ref is missing, or
a package could not be read on one side, which then is not compared.

Examples:
  gotest audit --base=origin/main
  gotest audit --record --output=base.json ./...
  gotest audit --base-record=base.json --fail-on=removed,focus,excluded --format=md
`)
}
