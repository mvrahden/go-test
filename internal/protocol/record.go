package protocol

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
}

// FailedUnit is one unit a run failed, at the finest level the runner can
// select again: a method ("Test<Suite>/<Method>"), a suite or fuzz wrapper
// ("Test<Suite>", "Fuzz<Suite>_<Method>") that failed without a failed
// method, or a whole package (Test empty), such as one that did not build.
type FailedUnit struct {
	Package string `json:"Package"`
	Test    string `json:"Test,omitempty"`
}
