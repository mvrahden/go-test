// Package gotestaudit reads a tree's test estate from source into a record,
// compares two records into an audit document and renders that document.
// Nothing here runs a test.
package gotestaudit

// Schema is the version of the record's and the audit document's shape. A
// field or a list entry added later keeps it; a field changed or removed
// bumps it.
const Schema = 1

// FloorNote says what a record's assertion counts are.
const FloorNote = "assertions counts the gotest assertion calls written in a body, a poll's call included and the calls in a poll's callback not; a helper's calls are not followed, so the count is a floor"

// Record is one tree's test estate as its source declares it.
type Record struct {
	// Version is the CLI that recorded it.
	Version    string     `json:"version"`
	Schema     int        `json:"schema"`
	Provenance Provenance `json:"provenance"`
	// Patterns are the packages recorded, as given.
	Patterns []string  `json:"patterns"`
	Notes    []string  `json:"notes"`
	Packages []Package `json:"packages"`
	// Suppressions are the nolint comments that can suppress a rule of
	// gotest lint, in lint -suppressions' order.
	Suppressions []Suppression `json:"suppressions"`
	Snapshots    []Snapshot    `json:"snapshots"`
	// Unread names what could not be read beyond a package, such as the
	// suppressions; a comparison then leaves it out.
	Unread []string `json:"unread,omitempty"`
}

// Provenance is the checkout a record was made from. A fact that could not be
// learned is left out.
type Provenance struct {
	// Ref is the ref the tree was exported from, "" for a working tree.
	Ref    string `json:"ref,omitempty"`
	Commit string `json:"commit,omitempty"`
	Dirty  *bool  `json:"dirty,omitempty"`
}

// Package is one package and the suites its test files declare.
type Package struct {
	Path   string `json:"path"`
	Module string `json:"module,omitempty"`
	// Dir is relative to the module root, slash-separated.
	Dir string `json:"dir"`
	// Unreadable holds why the package could not be read; it then lists no
	// suite, and nothing of it is compared.
	Unreadable []string `json:"unreadable,omitempty"`
	Suites     []Suite  `json:"suites"`
}

// Suite is one suite type.
type Suite struct {
	// Name is the type's name as written, X_ or F_ included.
	Name     string `json:"name"`
	Display  string `json:"display"`
	External bool   `json:"external"`
	Location
	Focused   bool   `json:"focused"`
	Excluded  bool   `json:"excluded"`
	Guard     bool   `json:"guard"`
	Parallel  bool   `json:"parallel"`
	Exclusive bool   `json:"exclusive"`
	Group     string `json:"group,omitempty"`
	Budget    Budget `json:"budget"`
	// Methods are its tests, benchmarks and fuzz targets, each with the
	// behaviors it declares.
	Methods []Behavior `json:"methods"`
}

// Budget is what a suite's source sets as its deadlines: a duration such as
// "30s", "none" for NoDeadline, or "unknown" where only a run knows it.
type Budget struct {
	Timeout      string `json:"timeout"`
	SetupTimeout string `json:"setupTimeout"`
}

// Budget values that are not durations.
const (
	BudgetNone    = "none"
	BudgetUnknown = "unknown"
)

// Behavior kinds.
const (
	KindTest      = "test"
	KindBenchmark = "benchmark"
	KindFuzz      = "fuzz"
	KindWhen      = "when"
	KindIt        = "it"
	KindRow       = "row"
	KindSuite     = "suite"
)

// Behavior is a method of a suite or a When, It or table row inside one.
type Behavior struct {
	Kind string `json:"kind"`
	// Name is the level go test reports for it, X_ or F_ included.
	Name    string `json:"name"`
	Display string `json:"display"`
	Location
	// Hash identifies its normalized body, the behaviors it declares left
	// out: tokens only, so layout and comments do not count.
	Hash string `json:"hash"`
	// Assertions counts the assertion calls its own body makes; see FloorNote.
	Assertions int  `json:"assertions"`
	AssertsBy  bool `json:"assertsBy"`
	Focused    bool `json:"focused"`
	Excluded   bool `json:"excluded"`
	// Incomplete says its body declares behaviors the source does not name,
	// so Children is a floor.
	Incomplete bool       `json:"incomplete,omitempty"`
	Children   []Behavior `json:"children"`
}

// Location is where the source declares something.
type Location struct {
	// File is relative to the module root, slash-separated.
	File    string `json:"file"`
	Line    int    `json:"line"`
	EndLine int    `json:"endLine"`
}

// Suppression is one nolint comment.
type Suppression struct {
	Module string   `json:"module,omitempty"`
	File   string   `json:"file"`
	Line   int      `json:"line"`
	Rules  []string `json:"rules"`
	Reason string   `json:"reason"`
}

// Snapshot is one file of MatchSnapshot's baselines.
type Snapshot struct {
	Module string `json:"module,omitempty"`
	File   string `json:"file"`
	SHA256 string `json:"sha256"`
}
