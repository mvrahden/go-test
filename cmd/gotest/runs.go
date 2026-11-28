package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/about"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/protocol"
)

var runsAllowed = flagSet("--format", "--import")

// runRuns lists the run records of the working directory, newest first,
// or with --import keeps a saved stream's run among them.
func runRuns(inv Invocation) int { //nolint:gocritic // hugeParam: stable API
	own, rest, err := SplitArgs(inv.Args, runsAllowed)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	if len(rest) > 0 {
		fmt.Fprintf(os.Stderr, "FAIL: runs takes no packages or go test flags: %s\n", strings.Join(rest, " "))
		return 2
	}
	format := own.ValueOr("--format", "text")
	if format != "text" && format != "json" {
		fmt.Fprintf(os.Stderr, "FAIL: unknown --format %q for runs: use text or json\n", format)
		return 2
	}
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	if own.Given("--import") {
		return importRun(own.Value("--import"), dir)
	}
	kept, err := gotestrunner.KeptRuns(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: reading the run records: %s\n", err)
		return 2
	}
	doc := runsDocument{Version: about.ResolvedVersion(), Dir: dir, Runs: []runsEntry{}}
	fresh := freshnessOf(context.Background(), kept)
	for i := range kept {
		doc.Runs = append(doc.Runs, entryOf(&kept[i], fresh))
	}
	if format == "json" {
		_ = json.NewEncoder(os.Stdout).Encode(doc)
		return 0
	}
	writeRuns(os.Stdout, &doc)
	return 0
}

// importRun keeps the run a saved stream holds among dir's records.
func importRun(file, dir string) int {
	if file == "" {
		fmt.Fprintln(os.Stderr, "FAIL: --import needs the events file of a run: --import=<file>")
		return 2
	}
	data, err := os.ReadFile(file) //nolint:gosec // G304: the stream the user names
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: --import: %s\n", err)
		return 2
	}
	rec, err := gotestrunner.RecordOfStream(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: --import: %s: %s\n", file, err)
		return 2
	}
	rec.Dir, rec.EventsFile = dir, absPath(file)
	if err := gotestrunner.ImportRun(rec, time.Now()); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: --import: %s\n", err)
		return 2
	}
	fmt.Fprintf(os.Stderr, "imported the run of %s with %d packages into the run records of %s\n", file, len(rec.Packages), dir)
	return 0
}

// runsDocument is what runs --format=json prints.
type runsDocument struct {
	Version string      `json:"version"`
	Dir     string      `json:"dir"`
	Runs    []runsEntry `json:"runs"`
}

type runsEntry struct {
	ID         string        `json:"id"`
	Started    *time.Time    `json:"started,omitempty"`
	Recorded   time.Time     `json:"recorded"`
	Imported   bool          `json:"imported,omitempty"`
	EventsFile string        `json:"eventsFile,omitempty"`
	Manifest   runsManifest  `json:"manifest"`
	Result     runsResult    `json:"result"`
	Failed     []runsUnit    `json:"failed"`
	Packages   runsFreshness `json:"packages"`
	// Ran are the packages the run judged, sorted; absent from a record
	// that kept none.
	Ran []runsPackage `json:"ran,omitzero"`
}

// runsPackage is one package a run judged.
type runsPackage struct {
	Package   string `json:"package"`
	Status    string `json:"status"`
	InputHash string `json:"inputHash,omitempty"`
}

type runsResult struct {
	ExitCode int       `json:"exitCode"`
	Packages runsTally `json:"packages"`
	// Tests is absent for a text run, which counts no tests.
	Tests   *runsTally `json:"tests,omitempty"`
	NotRun  *int       `json:"notRun,omitempty"`
	Vacuous *int       `json:"vacuous,omitempty"`
}

type runsTally struct {
	Passed  int `json:"passed"`
	Failed  int `json:"failed"`
	Skipped int `json:"skipped"`
}

type runsUnit struct {
	Package string `json:"package"`
	Test    string `json:"test,omitempty"`
}

type runsManifest struct {
	Version   string       `json:"version,omitempty"`
	GoVersion string       `json:"goVersion,omitempty"`
	Args      []string     `json:"args"`
	Commit    string       `json:"commit,omitempty"`
	Dirty     *bool        `json:"dirty,omitempty"`
	Agent     string       `json:"agent,omitempty"`
	Build     []string     `json:"build,omitempty"`
	Changed   *runsChanged `json:"changed,omitempty"`
}

// runsChanged is how --changed narrowed a run, keyed as spec's document
// keys it.
type runsChanged struct {
	Ran       int  `json:"ran"`
	Unchanged int  `json:"unchanged"`
	NoRecord  bool `json:"noRecord,omitempty"`
	// Packages are the import paths it ran, sorted; absent from a record
	// older than the list.
	Packages []string `json:"packages,omitzero"`
}

// changedOf is c as the listing prints it, nil for a run not narrowed.
func changedOf(c *protocol.Changed) *runsChanged {
	if c == nil {
		return nil
	}
	return &runsChanged{Ran: c.Ran, Unchanged: c.Unchanged, NoRecord: c.NoRecord, Packages: c.Packages}
}

// runsFreshness counts a record's packages by whether their verdict still
// belongs to the working tree.
type runsFreshness struct {
	Total   int `json:"total"`
	Current int `json:"current"`
	Stale   int `json:"stale"`
	Unknown int `json:"unknown"`
}

func entryOf(k *gotestrunner.KeptRun, fresh map[string]map[string]string) runsEntry {
	r := &k.Record
	m := r.Manifest
	e := runsEntry{
		ID: k.ID, Recorded: r.Time, Imported: r.Imported, EventsFile: r.EventsFile,
		Manifest: runsManifest{Version: m.Version, GoVersion: m.GoVersion, Args: m.Args, Commit: m.Commit, Dirty: m.Dirty, Agent: m.Agent, Build: m.Build, Changed: changedOf(m.Changed)},
		Result: runsResult{
			ExitCode: r.Result.ExitCode, NotRun: r.Result.NotRun, Vacuous: r.Result.Vacuous,
			Packages: runsTally(r.Result.Packages),
		},
		Failed: []runsUnit{},
	}
	for _, u := range r.Failed {
		e.Failed = append(e.Failed, runsUnit(u))
	}
	if t := r.Result.Tests; t != nil {
		tests := runsTally(*t)
		e.Result.Tests = &tests
	}
	for _, p := range r.Packages {
		e.Ran = append(e.Ran, runsPackage{Package: p.Package, Status: p.Verdict, InputHash: p.InputHash})
	}
	slices.SortStableFunc(e.Ran, func(a, b runsPackage) int { return strings.Compare(a.Package, b.Package) })
	if e.Manifest.Args == nil {
		e.Manifest.Args = []string{}
	}
	if !r.Started.IsZero() {
		started := r.Started
		e.Started = &started
	}
	current := fresh[buildKey(m.Build)]
	for _, p := range r.Packages {
		e.Packages.Total++
		h, ok := current[p.Package]
		switch {
		case p.InputHash == "":
			e.Packages.Unknown++
		case ok && h == p.InputHash:
			e.Packages.Current++
		default:
			e.Packages.Stale++
		}
	}
	return e
}

func buildKey(build []string) string { return strings.Join(build, "\x00") }

// freshnessOf hashes the working tree's packages the records name, once
// per set of build flags they ran under. A load that fails judges its
// packages stale.
func freshnessOf(ctx context.Context, kept []gotestrunner.KeptRun) map[string]map[string]string {
	pkgs := map[string]map[string]bool{}
	builds := map[string][]string{}
	for i := range kept {
		r := &kept[i].Record
		key := buildKey(r.Manifest.Build)
		builds[key] = r.Manifest.Build
		if pkgs[key] == nil {
			pkgs[key] = map[string]bool{}
		}
		for _, p := range r.Packages {
			if p.InputHash != "" {
				pkgs[key][p.Package] = true
			}
		}
	}
	out := map[string]map[string]string{}
	for key, set := range pkgs {
		var list []string
		for p := range set {
			list = append(list, p)
		}
		hashes, err := currentHashes(ctx, list, builds[key])
		if err != nil {
			fmt.Fprintf(os.Stderr, "gotest: runs: the working tree's packages could not be hashed: %s\n", err)
			continue
		}
		out[key] = hashes
	}
	return out
}

// runWord is the closing line's word for a run: FAIL for a non-zero exit
// code, EMPTY when nothing passed, counting tests where the record holds
// them and packages otherwise, else PASS.
func runWord(r *runsResult) string {
	passed := r.Packages.Passed
	if r.Tests != nil {
		passed = r.Tests.Passed
	}
	switch {
	case r.ExitCode != 0:
		return "FAIL"
	case passed == 0:
		return "EMPTY"
	}
	return "PASS"
}

// writeRuns lists the records as text, one line each, newest first.
func writeRuns(w io.Writer, doc *runsDocument) {
	if len(doc.Runs) == 0 {
		fmt.Fprintf(w, "no run of %s is recorded\n", doc.Dir)
		return
	}
	for i := range doc.Runs {
		e := &doc.Runs[i]
		verdict := runWord(&e.Result)
		parts := []string{
			e.Recorded.Local().Format("2006-01-02 15:04:05"),
			fmt.Sprintf("%s exit %d", verdict, e.Result.ExitCode),
			fmt.Sprintf("packages: %d current, %d stale, %d unknown", e.Packages.Current, e.Packages.Stale, e.Packages.Unknown),
		}
		if c := e.Manifest.Commit; c != "" {
			if d := e.Manifest.Dirty; d != nil && *d {
				c += " dirty"
			}
			parts = append(parts, c)
		}
		if e.Manifest.Agent != "" {
			parts = append(parts, e.Manifest.Agent)
		}
		if e.Imported {
			parts = append(parts, "imported "+filepath.Base(e.EventsFile))
		}
		parts = append(parts, strings.TrimSpace("gotest "+strings.Join(e.Manifest.Args, " ")))
		fmt.Fprintln(w, strings.Join(parts, " | "))
	}
}
