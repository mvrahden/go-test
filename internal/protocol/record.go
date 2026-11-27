package protocol

import "time"

// RunRecord is what a run leaves for the next one in its working directory:
// what it was, what it judged and the units that failed, which --failed
// reruns.
type RunRecord struct {
	// Dir is the working directory the run was made in.
	Dir      string    `json:"Dir"`
	Manifest Manifest  `json:"Manifest"`
	Result   RunResult `json:"Result"`
	// Failed lists the failed units in the order the run judged them.
	Failed []FailedUnit `json:"Failed"`
	// Started is when the run began; Time when the record was written.
	Started time.Time `json:"Started,omitzero"`
	Time    time.Time `json:"Time,omitzero"`
	// Packages are the verdicts of the packages the run judged, in order.
	Packages []PackageVerdict `json:"Packages,omitempty"`
	// EventsFile is the file the run wrote its stream to (--events-file),
	// or the stream an import read.
	EventsFile string `json:"EventsFile,omitempty"`
	// Imported marks a record read from a saved stream, not left by a run.
	Imported bool `json:"Imported,omitempty"`
}

// PackageVerdict is one package's verdict in a run's record.
type PackageVerdict struct {
	Package string `json:"Package"`
	// Verdict is pass, fail or skip.
	Verdict string `json:"Verdict"`
	// InputHash is the hash the verdict was stamped with; "" for none.
	InputHash string `json:"InputHash,omitempty"`
	// Elapsed is the package's time in seconds.
	Elapsed float64 `json:"Elapsed"`
	// Narrowed marks a verdict that does not stand for all the package's
	// tests: the run selected among them, did not count them, or failed
	// outside any package.
	Narrowed bool `json:"Narrowed,omitempty"`
	// Mode is the mode the verdict was taken under; nil in a record
	// written before modes, which covers no run.
	Mode *VerdictMode `json:"Mode,omitempty"`
}

// VerdictMode is what a run's flags and environment make of a verdict
// beyond the source it judged.
type VerdictMode struct {
	// Policy is CI mode: its gates judged the verdict.
	Policy bool `json:"Policy"`
	// Short is -short, or -test.short after -args.
	Short bool `json:"Short"`
}

// Covers reports whether a verdict under m stands for a run under run:
// the same Short, and Policy on or the run's off. A nil mode covers none.
func (m *VerdictMode) Covers(run VerdictMode) bool {
	return m != nil && m.Short == run.Short && (m.Policy || !run.Policy)
}

// FailedUnit is one unit a run failed, at the finest level the runner can
// select again: a method ("Test<Suite>/<Method>"), a suite or fuzz wrapper
// ("Test<Suite>", "Fuzz<Suite>_<Method>") that failed without a failed
// method, or a whole package (Test empty), such as one that did not build.
type FailedUnit struct {
	Package string `json:"Package"`
	Test    string `json:"Test,omitempty"`
}
