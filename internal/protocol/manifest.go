package protocol

// RunPackage is the package a -json run opens and closes itself under: its
// start event carries the Manifest, its verdict the RunResult. The space
// keeps it out of the import-path namespace.
const RunPackage = "gotest run"

// Manifest says what a run was: the CLI and the Go that ran it, its
// arguments, the checkout and the agent it ran under. A fact the run could
// not learn is left out.
type Manifest struct {
	Version   string   `json:"Version"`
	GoVersion string   `json:"GoVersion,omitempty"`
	Args      []string `json:"Args"`
	Commit    string   `json:"Commit,omitempty"`
	// Dirty is set only in a checkout: whether it has uncommitted changes.
	Dirty *bool  `json:"Dirty,omitempty"`
	Agent string `json:"Agent,omitempty"`
}

// RunResult is what a run judged: its exit code and the verdicts its stream
// holds, the run's own package aside.
type RunResult struct {
	ExitCode int `json:"ExitCode"`
	// Packages counts each package by its last verdict; Tests counts every
	// test verdict, subtests included.
	Packages Tally `json:"Packages"`
	Tests    Tally `json:"Tests"`
	// NotRun is how many declared units the census found without a verdict;
	// absent when the census did not judge the run.
	NotRun *int `json:"NotRun,omitempty"`
	// Vacuous is how many tests passed without an assertion and declared no
	// other means; absent when no test's runtime reported a count.
	Vacuous *int `json:"Vacuous,omitempty"`
}

// Tally counts verdicts.
type Tally struct {
	Passed  int `json:"Passed"`
	Failed  int `json:"Failed"`
	Skipped int `json:"Skipped"`
}

// Add counts one verdict; another action counts nothing.
func (t *Tally) Add(action string) {
	switch action {
	case "pass":
		t.Passed++
	case "fail":
		t.Failed++
	case "skip":
		t.Skipped++
	}
}
