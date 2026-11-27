package gotestaudit

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"slices"
	"strings"
)

// Document is the audit of a head tree against a base: what the suites
// declare now against before, and what got weaker.
type Document struct {
	// Version is the CLI that compared the records.
	Version string `json:"version"`
	Schema  int    `json:"schema"`
	Base    Side   `json:"base"`
	Head    Side   `json:"head"`
	// Modules are the module paths the records hold, for shortening a
	// package to its path in its module.
	Modules []string `json:"modules"`
	// ModuleDirs says where each module lies, relative to the directory
	// the head was recorded in.
	ModuleDirs []ModuleDir `json:"moduleDirs"`
	Summary    Summary     `json:"summary"`
	// Findings come in the order of Classes, the weakenings first: the
	// order the text format lists them in.
	Findings []Finding `json:"findings"`
	// Behaviors come by change; each one's Order is its place in the text
	// format's list.
	Behaviors []Change `json:"behaviors"`
	// Streams are the runs given as evidence, in the order given; an
	// Evidence names one by its index.
	Streams []Stream `json:"streams"`
	// Evidence holds what the head's streams say of each behavior the
	// comparison names; empty without a stream.
	Evidence []Evidence `json:"evidence"`
	// Coverage says where the unproven lines were read from; absent when
	// they were not looked for.
	Coverage *Coverage `json:"coverage,omitempty"`
	// Unproven holds, per production file, the changed lines no test
	// executed, and the files no profile covers.
	Unproven []Unproven `json:"unproven"`
	// Mutation is what --mutate found of the changed lines; absent without
	// it.
	Mutation *Mutation `json:"mutation,omitempty"`
	// Incomplete names what could not be compared, such as a package that
	// does not compile on one side.
	Incomplete []string `json:"incomplete"`
	Notes      []string `json:"notes"`
}

// Side is one record a document compares.
type Side struct {
	Version string `json:"version"`
	Provenance
	Patterns []string `json:"patterns"`
}

// Summary counts the document.
type Summary struct {
	Weakenings    int `json:"weakenings"`
	Informational int `json:"informational"`
	// Classes counts, per class that has any, the findings --fail-on
	// judges: the weakenings and the changed bodies.
	Classes  map[string]int `json:"classes"`
	Added    int            `json:"added"`
	Removed  int            `json:"removed"`
	Reworded int            `json:"reworded"`
	Moved    int            `json:"moved"`
	Changed  int            `json:"changed"`
	Evidence int            `json:"evidence"`
	// Unproven counts the changed executable lines no test executed.
	Unproven   int `json:"unproven"`
	Incomplete int `json:"incomplete"`
	// Failing, NotRun, Vacuous and Stale count the evidence entries of
	// each kind (see the proof classes).
	Failing int `json:"failing"`
	NotRun  int `json:"notRun"`
	Vacuous int `json:"vacuous"`
	Stale   int `json:"stale"`
	// ChangedLines counts the changed lines a profile knows as executable;
	// NoCoverage the changed production files no profile covers.
	ChangedLines int `json:"changedLines"`
	NoCoverage   int `json:"noCoverage"`
	// Mutation counts the mutants by verdict; absent without --mutate.
	Mutation *MutationCounts `json:"mutation,omitempty"`
}

// Change kinds.
const (
	ChangeAdded    = "added"
	ChangeRemoved  = "removed"
	ChangeReworded = "reworded"
	ChangeMoved    = "moved"
	ChangeChanged  = "changed"
)

// Change is one behavior that is not as it was.
type Change struct {
	Change  string `json:"change"`
	Kind    string `json:"kind"`
	Package string `json:"package"`
	Module  string `json:"module,omitempty"`
	Base    *Place `json:"base,omitempty"`
	Head    *Place `json:"head,omitempty"`
	// Behaviors counts the leaves of an added or removed one, itself when it
	// is a leaf.
	Behaviors int `json:"behaviors,omitempty"`
	Selector
	// Evidence is the id of the evidence row about it; empty for none.
	Evidence string `json:"evidence,omitempty"`
	// Order is its place in the text format's list of behaviors, from 0.
	Order int `json:"order"`
}

// Selector names the test go test runs for a behavior: its full name and
// the -run pattern that selects it, in Package. Empty for a benchmark, a
// fuzz target or what lies under one.
type Selector struct {
	Test string `json:"test,omitempty"`
	Run  string `json:"run,omitempty"`
}

// selectorOf is the selector of the test whose levels are names.
func selectorOf(names []string, runnable bool) Selector {
	if !runnable || len(names) == 0 {
		return Selector{}
	}
	levels := make([]string, len(names))
	for i, n := range names {
		levels[i] = "^" + regexp.QuoteMeta(n) + "$"
	}
	return Selector{Test: strings.Join(names, "/"), Run: strings.Join(levels, "/")}
}

// ModuleDir is a module and its directory, slash-separated, "." for the
// directory recorded in.
type ModuleDir struct {
	Path string `json:"path"`
	Dir  string `json:"dir"`
}

// stableID is a stable id of the parts that name something.
func stableID(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])[:16]
}

// Place is a behavior's labels, from its suite down, and where it is declared.
type Place struct {
	Path []string `json:"path"`
	Location
}

// Finding classes, in the order a document lists them.
const (
	ClassRemoved     = "removed"
	ClassExcluded    = "excluded"
	ClassSkip        = "skip"
	ClassFocus       = "focus"
	ClassGuard       = "guard"
	ClassBudget      = "budget"
	ClassSuppression = "suppression"
	ClassSnapshot    = "snapshot"
	ClassAssertions  = "assertions"
	ClassAssertsBy   = "asserts-by"
	ClassChanged     = "changed"
)

// Classes are the finding classes in order; all but changed weaken.
var Classes = []string{
	ClassRemoved, ClassExcluded, ClassSkip, ClassFocus, ClassGuard, ClassBudget,
	ClassSuppression, ClassSnapshot, ClassAssertions, ClassAssertsBy, ClassChanged,
}

// Proof classes judge what the runs and the profile given say, not the
// change: a behavior that failed, did not run, passed vacuous or was
// judged by a stale stream, a file with changed lines no test executed,
// and a line with a mutant no test noticed (ClassSurvived).
const (
	ClassFailing  = "failing"
	ClassNotRun   = "not-run"
	ClassVacuous  = "vacuous"
	ClassStale    = "stale"
	ClassUnproven = "unproven"
)

// ProofClasses are the proof classes in order.
var ProofClasses = []string{ClassFailing, ClassNotRun, ClassVacuous, ClassStale, ClassUnproven, ClassSurvived}

// Weakens reports whether a class is a weakening, not information. A
// suppression that only changed its reason is information of its class.
func Weakens(class string) bool { return class != ClassChanged && slices.Contains(Classes, class) }

// Finding is one fact of a class: what it is about and its value on each
// side.
type Finding struct {
	// ID is made from what the finding is about: its class, module,
	// package, subject and values, never its order or lines.
	ID      string `json:"id"`
	Class   string `json:"class"`
	Weakens bool   `json:"weakens"`
	Package string `json:"package,omitempty"`
	// Subject is a behavior's labels joined by " / ", or a file.
	Subject string `json:"subject"`
	Base    string `json:"base"`
	Head    string `json:"head"`
	// Module is the module of the package or the file it is about.
	Module string `json:"module,omitempty"`
	// Location is the head's, the base's for what the head no longer has.
	Location *Location `json:"location,omitempty"`
	// BaseLocation and HeadLocation are each side's, where it has one.
	BaseLocation *Location `json:"baseLocation,omitempty"`
	HeadLocation *Location `json:"headLocation,omitempty"`
	Selector
	// Runtime is the assertion count each side's run reported, beside the
	// static floor in Base and Head; on an assertions finding only.
	Runtime *Runtime `json:"runtime,omitempty"`
	// Evidence is the id of the evidence row about its behavior; empty for
	// none.
	Evidence string `json:"evidence,omitempty"`

	// The labels of the behavior on each side, nil where it has none.
	basePath, headPath []string
}

// Runtime is a count each side's stream reported; nil where none did.
type Runtime struct {
	Base *int `json:"base"`
	Head *int `json:"head"`
}

// Stream sides.
const (
	SideHead = "head"
	SideBase = "base"
)

// Stream is one run given as evidence, as its manifest describes it.
type Stream struct {
	// File is the stream's file as given; empty for the run the audit
	// made itself (Ran).
	File string `json:"file,omitempty"`
	Side string `json:"side"`
	Ran  bool   `json:"ran,omitempty"`
	// Run is the stream's manifest and exit code; nil for a stream that
	// opens with no manifest.
	Run *StreamRun `json:"run"`
	// Current says the stream ran the side's tree: the audit's own run;
	// one whose packages' input hashes the head's tree has each; else one
	// of the side's commit, with neither dirty. Stale says why not.
	Current bool   `json:"current"`
	Stale   string `json:"stale,omitempty"`
}

// StreamRun is what a stream's first and last events say of its run.
type StreamRun struct {
	Version   string   `json:"version,omitempty"`
	GoVersion string   `json:"goVersion,omitempty"`
	Args      []string `json:"args,omitempty"`
	Commit    string   `json:"commit,omitempty"`
	Dirty     *bool    `json:"dirty,omitempty"`
	Agent     string   `json:"agent,omitempty"`
	// ExitCode is nil for a stream the run did not close.
	ExitCode *int `json:"exitCode,omitempty"`
}

// Evidence verdicts beside pass, fail and skip.
const (
	VerdictPass = "pass"
	VerdictFail = "fail"
	VerdictSkip = "skip"
	// VerdictNotRun: a stream holds the behavior's package, not it.
	VerdictNotRun = "not run"
	// VerdictNone: no stream holds the behavior's package.
	VerdictNone = "no evidence"
)

// Evidence is what the head's streams say of one behavior the comparison
// names: one added, reworded, moved or changed, or a finding's subject.
type Evidence struct {
	// ID is made from the behavior's package and labels.
	ID      string   `json:"id"`
	Package string   `json:"package"`
	Module  string   `json:"module,omitempty"`
	Path    []string `json:"path"`
	// Location is where the head declares the behavior.
	Location *Location `json:"location,omitempty"`
	Verdict  string    `json:"verdict"`
	// Mark is the row's mark in the text format: its verdict, a passing
	// leaf's assertions, * for a stale stream.
	Mark string `json:"mark"`
	Kind string `json:"kind"`
	// Why are its change and the classes of the findings about it.
	Why []string `json:"why"`
	// Assertions is the count the run reported: a leaf's own, or the sum
	// over the leaves beneath that ran, when each of them reported one.
	Assertions *int   `json:"assertions,omitempty"`
	Vacuous    bool   `json:"vacuous,omitempty"`
	AssertsBy  string `json:"assertsBy,omitempty"`
	SkipReason string `json:"skipReason,omitempty"`
	// Runs counts the runs of a stream that repeated it (-count).
	Runs int `json:"runs,omitempty"`
	// Leaves tallies the leaves beneath a behavior that has any.
	Leaves *Leaves `json:"leaves,omitempty"`
	// Stream is the index of the stream judged by; nil for no evidence.
	Stream  *int   `json:"stream,omitempty"`
	Current bool   `json:"current"`
	Stale   string `json:"stale,omitempty"`
	// Others are the other streams that hold a verdict on it.
	Others []OtherVerdict `json:"others,omitempty"`
	// NeedsDecision marks a row a reader must judge: failing, vacuous,
	// not run, partly run, without evidence, or judged by a stale stream.
	NeedsDecision bool `json:"needsDecision"`
	// Source is the judging stream's file, "--run" for the audit's run;
	// Commit the commit that stream ran.
	Source string `json:"source,omitempty"`
	Commit string `json:"commit,omitempty"`
	Selector
}

// Leaves counts the verdicts of the leaves beneath a behavior.
type Leaves struct {
	Passed  int `json:"passed"`
	Failed  int `json:"failed"`
	Skipped int `json:"skipped"`
	NotRun  int `json:"notRun"`
	Vacuous int `json:"vacuous"`
}

// OtherVerdict is another stream's verdict on a behavior.
type OtherVerdict struct {
	Stream  int    `json:"stream"`
	Verdict string `json:"verdict"`
	Current bool   `json:"current"`
}

// Coverage is what the unproven lines were read from.
type Coverage struct {
	// Profile is the coverage profile's file; empty for the audit's run.
	Profile string `json:"profile,omitempty"`
	// Lines names where the changed lines came from: a diff file, or the
	// git diff of the base and the working tree.
	Lines string `json:"lines"`
	// Packages are the production packages the profile covers among the
	// changed ones.
	Packages []string `json:"packages"`
}

// Unproven is one changed production file: its changed lines a profile
// knows as executable and which of them no test executed. A file whose
// package no profile covers has Coverage false and no lines.
type Unproven struct {
	Module string `json:"module,omitempty"`
	// File is relative to the module root, slash-separated.
	File    string `json:"file"`
	Package string `json:"package"`
	// Line is the first changed line no test executed, 0 for none.
	Line     int  `json:"line"`
	Coverage bool `json:"coverage"`
	// Changed counts the changed executable lines; Unexecuted those of
	// them no test executed, in Ranges.
	Changed    int         `json:"changed"`
	Unexecuted int         `json:"unexecuted"`
	Ranges     []LineRange `json:"ranges"`
}

// LineRange is the lines from Line to EndLine, both included.
type LineRange struct {
	Line    int `json:"line"`
	EndLine int `json:"endLine"`
}
