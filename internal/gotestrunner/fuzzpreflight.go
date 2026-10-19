package gotestrunner

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

// CorpusMismatch is one recorded corpus entry whose value shape no longer
// matches the target that owns it.
type CorpusMismatch struct {
	Func string   // generated fuzz func name
	File string   // path of the corpus entry, relative to the package directory
	Got  []string // corpus types the entry holds
	Want []string // corpus types the target now takes
}

// Message is the warning to print for one stale entry. It names the drift and
// both ways out: an input worth keeping becomes a typed seed by hand, since
// promote takes only the latest session's crashers, and one that only ever
// mattered to the old shape is noise.
func (m *CorpusMismatch) Message() string {
	return fmt.Sprintf("fuzz: %s: %s has %d values of [%s], but the target now takes %d [%s] — it predates a change to the fuzzed type's fields; delete it, or write its input as a typed f.Add seed",
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
