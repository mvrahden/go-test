package gotestaudit

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
	Summary Summary  `json:"summary"`
	// Findings come in the order of Classes, the weakenings first.
	Findings  []Finding `json:"findings"`
	Behaviors []Change  `json:"behaviors"`
	// Evidence holds what runs proved of the changed behaviors; this
	// version fills none.
	Evidence []Evidence `json:"evidence"`
	// Unproven holds the changed production lines no test executed; this
	// version fills none.
	Unproven []Unproven `json:"unproven"`
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
	Classes    map[string]int `json:"classes"`
	Added      int            `json:"added"`
	Removed    int            `json:"removed"`
	Reworded   int            `json:"reworded"`
	Moved      int            `json:"moved"`
	Changed    int            `json:"changed"`
	Evidence   int            `json:"evidence"`
	Unproven   int            `json:"unproven"`
	Incomplete int            `json:"incomplete"`
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
	Base    *Place `json:"base,omitempty"`
	Head    *Place `json:"head,omitempty"`
	// Behaviors counts the leaves of an added or removed one, itself when it
	// is a leaf.
	Behaviors int `json:"behaviors,omitempty"`
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
	ClassRemoved, ClassExcluded, ClassFocus, ClassGuard, ClassBudget,
	ClassSuppression, ClassSnapshot, ClassAssertions, ClassAssertsBy, ClassChanged,
}

// Weakens reports whether a class is a weakening, not information. A
// suppression that only changed its reason is information of its class.
func Weakens(class string) bool { return class != ClassChanged }

// Finding is one fact of a class: what it is about and its value on each
// side.
type Finding struct {
	Class   string `json:"class"`
	Weakens bool   `json:"weakens"`
	Package string `json:"package,omitempty"`
	// Subject is a behavior's labels joined by " / ", or a file.
	Subject string `json:"subject"`
	Base    string `json:"base"`
	Head    string `json:"head"`
	// Location is the head's, the base's for what the head no longer has.
	Location *Location `json:"location,omitempty"`
}

// Evidence is a run's verdict on a changed behavior.
type Evidence struct {
	Package string   `json:"package"`
	Path    []string `json:"path"`
	Verdict string   `json:"verdict"`
}

// Unproven is a changed production line no test executed.
type Unproven struct {
	Module string `json:"module,omitempty"`
	File   string `json:"file"`
	Line   int    `json:"line"`
}
