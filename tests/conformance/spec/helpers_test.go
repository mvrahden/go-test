package spec_test

import (
	"encoding/json"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// specDoc is what the spec document's consumers read of it.
type specDoc struct {
	Packages []specPackage `json:"packages"`
	Stats    specStats     `json:"stats"`
}

type specPackage struct {
	Path   string     `json:"path"`
	Dir    string     `json:"dir"`
	Status string     `json:"status"`
	Nodes  []specNode `json:"nodes"`
	Output []string   `json:"output"`
	Stats  *specStats `json:"stats"`
}

type specNode struct {
	Name       string     `json:"name"`
	Display    string     `json:"display"`
	Status     string     `json:"status"`
	Duration   float64    `json:"duration"`
	Incomplete bool       `json:"incomplete"`
	File       string     `json:"file"`
	Line       int        `json:"line"`
	Children   []specNode `json:"children"`
}

type specStats struct {
	Behaviors int `json:"behaviors"`
	Passed    int `json:"passed"`
	Failed    int `json:"failed"`
	Skipped   int `json:"skipped"`
	NotRun    int `json:"notRun"`
	// FailedPackages counts packages that failed on their own.
	FailedPackages int `json:"failedPackages"`
}

// spec runs gotest spec from this directory and fails t unless it exits as
// want.
func spec(t *gotest.T, want int, args ...string) testkit.Run {
	run, err := testkit.RunCLI(t.Context(), ".", append([]string{"spec", "--no-color"}, args...)...)
	gotest.NoError(t, err)
	gotest.Equal(t, want, run.Exit, "stderr: %s", run.Stderr)
	return run
}

// specJSON runs gotest spec --format=json and reads the document.
func specJSON(t *gotest.T, want int, args ...string) specDoc {
	run := spec(t, want, append([]string{"--format=json"}, args...)...)
	var doc specDoc
	gotest.NoError(t, json.Unmarshal([]byte(run.Stdout), &doc), "stdout: %s", run.Stdout)
	return doc
}

// node finds the node at path, one name per level, among nodes.
func node(t *gotest.T, nodes []specNode, path ...string) specNode {
	var found specNode
	for _, name := range path {
		ok := false
		for _, n := range nodes {
			if n.Name == name {
				found, nodes, ok = n, n.Children, true
				break
			}
		}
		gotest.True(t, ok, "no node %q on the path %v", name, path)
	}
	return found
}
