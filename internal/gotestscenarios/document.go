package gotestscenarios

import (
	"encoding/json"
	"io"

	"github.com/mvrahden/go-test/internal/about"
	"github.com/mvrahden/go-test/internal/gotestspec"
)

// Schema is the document's shape; it grows only by adding fields.
const Schema = 1

// What a document's verdicts come from.
const (
	EvidenceNone  = "none"
	EvidenceInput = "input"
	EvidenceRun   = "run"
)

// Statuses of a scenario and of a clause. Bound is a binding without
// evidence; verified, failing, not-run and vacuous are what evidence says
// of a bound one.
const (
	StatusBound     = "bound"
	StatusUnbound   = "unbound"
	StatusAmbiguous = "ambiguous"
	StatusVerified  = "verified"
	StatusFailing   = "failing"
	StatusNotRun    = "not-run"
	StatusVacuous   = "vacuous"
)

// Document is every scenario of the specs read, bound to the tree's Its.
type Document struct {
	// Version is the CLI that wrote the document.
	Version  string `json:"version"`
	Schema   int    `json:"schema"`
	Evidence string `json:"evidence"`
	// Run is the run the verdicts come from, as its stream names it.
	Run       *Run       `json:"run,omitempty"`
	Specs     []string   `json:"specs"`
	Patterns  []string   `json:"patterns"`
	Summary   Summary    `json:"summary"`
	Scenarios []Scenario `json:"scenarios"`
	// Unstated holds the Its under bound Whens no clause states; listed only
	// when asked for.
	Unstated []Place `json:"unstated"`
	// Incomplete names the methods whose behaviors source cannot list: a
	// scenario they might state reads as unbound.
	Incomplete []string `json:"incomplete"`
	// Unreadable names the packages that could not be read.
	Unreadable []string `json:"unreadable"`
}

// Run is the run a stream names in its manifest.
type Run struct {
	Version   string   `json:"version,omitempty"`
	GoVersion string   `json:"goVersion,omitempty"`
	Args      []string `json:"args,omitempty"`
	Commit    string   `json:"commit,omitempty"`
	Dirty     *bool    `json:"dirty,omitempty"`
	Agent     string   `json:"agent,omitempty"`
	ExitCode  *int     `json:"exitCode,omitempty"`
	// Packages are the packages the stream reports.
	Packages []string `json:"packages"`
	// Failed says the run --verify made exited non-zero.
	Failed bool `json:"failed,omitempty"`
}

// Summary counts the scenarios by status.
type Summary struct {
	Scenarios int            `json:"scenarios"`
	Statuses  map[string]int `json:"statuses"`
	Unstated  int            `json:"unstated"`
}

// Scenario is one scenario and what binds it.
type Scenario struct {
	Capability  string `json:"capability"`
	File        string `json:"file"`
	Requirement string `json:"requirement"`
	Title       string `json:"title"`
	Line        int    `json:"line"`
	Status      string `json:"status"`
	// Problem says why the scenario as written cannot bind.
	Problem    string   `json:"problem,omitempty"`
	Conditions []Clause `json:"conditions"`
	// Whens are the innermost Whens whose labels state the conditions.
	Whens   []Place  `json:"whens"`
	Clauses []Clause `json:"clauses"`
}

// Clause is a GIVEN, WHEN, THEN or AND as written, and for an outcome the
// Its that state it.
type Clause struct {
	Keyword string  `json:"keyword"`
	Text    string  `json:"text"`
	Line    int     `json:"line"`
	Status  string  `json:"status,omitempty"`
	Its     []Place `json:"its,omitempty"`

	its    []*it
	shared bool
}

// Place is a When or It declaration: its package, module-relative file and
// line, and its path from the suite down.
type Place struct {
	Package string   `json:"package"`
	File    string   `json:"file"`
	Line    int      `json:"line"`
	Path    []string `json:"path"`
	Status  string   `json:"status,omitempty"`
	// Assertions is the fewest assertions a run of the It made.
	Assertions *int `json:"assertions,omitempty"`
}

func newDocument(opt *Options) Document {
	doc := Document{
		Version:    about.ResolvedVersion(),
		Schema:     Schema,
		Evidence:   opt.Evidence,
		Specs:      []string{},
		Patterns:   append([]string{}, opt.Patterns...),
		Scenarios:  []Scenario{},
		Unstated:   []Place{},
		Incomplete: append([]string{}, opt.Incomplete...),
		Unreadable: append([]string{}, opt.Unreadable...),
	}
	if doc.Evidence == "" {
		doc.Evidence = EvidenceNone
	}
	if opt.Run != nil {
		doc.Run = runOf(opt.Run)
		doc.Run.Packages = append([]string{}, opt.RunPackages...)
		doc.Run.Failed = opt.Evidence == EvidenceRun && opt.Run.ExitCode != nil && *opt.Run.ExitCode != 0
	}
	return doc
}

func runOf(r *gotestspec.RunInfo) *Run {
	m := r.Manifest
	return &Run{Version: m.Version, GoVersion: m.GoVersion, Args: m.Args, Commit: m.Commit, Dirty: m.Dirty, Agent: m.Agent, ExitCode: r.ExitCode}
}

func (d *Document) summarize() {
	d.Summary = Summary{Scenarios: len(d.Scenarios), Statuses: map[string]int{}, Unstated: len(d.Unstated)}
	for i := range d.Scenarios {
		d.Summary.Statuses[d.Scenarios[i].Status]++
	}
}

// WriteJSON writes the document, indented, with a final newline.
func WriteJSON(w io.Writer, d *Document) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(d)
}
