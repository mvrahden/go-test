package scenarios_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// stage copies the scenarios module into a fresh directory under parent
// and returns it.
func stage(t *gotest.T, parent, name string) string {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	dir := filepath.Join(parent, name)
	gotest.NoError(t, testkit.StageModule(root, filepath.Join("..", "testdata", "scenarios"), dir))
	return dir
}

// scenarios runs gotest scenarios in dir with args.
func scenarios(t *gotest.T, dir string, args ...string) testkit.Run {
	run, err := testkit.RunCLI(t.Context(), dir, append([]string{"scenarios"}, args...)...)
	gotest.NoError(t, err)
	return run
}

// document is the part of a scenarios document the suites read.
type document struct {
	Version  string `json:"version"`
	Schema   int    `json:"schema"`
	Evidence string `json:"evidence"`
	Run      *struct {
		Version  string   `json:"version"`
		Args     []string `json:"args"`
		ExitCode *int     `json:"exitCode"`
		Packages []string `json:"packages"`
		Failed   bool     `json:"failed"`
	} `json:"run"`
	Specs   []string `json:"specs"`
	Summary struct {
		Scenarios int            `json:"scenarios"`
		Statuses  map[string]int `json:"statuses"`
		Unstated  int            `json:"unstated"`
	} `json:"summary"`
	Scenarios  []scenario `json:"scenarios"`
	Unstated   []place    `json:"unstated"`
	Unreadable []string   `json:"unreadable"`
}

type scenario struct {
	Capability  string   `json:"capability"`
	File        string   `json:"file"`
	Requirement string   `json:"requirement"`
	Title       string   `json:"title"`
	Line        int      `json:"line"`
	Status      string   `json:"status"`
	Conditions  []clause `json:"conditions"`
	Whens       []place  `json:"whens"`
	Clauses     []clause `json:"clauses"`
}

type clause struct {
	Keyword string  `json:"keyword"`
	Text    string  `json:"text"`
	Line    int     `json:"line"`
	Status  string  `json:"status"`
	Its     []place `json:"its"`
}

type place struct {
	Package string   `json:"package"`
	File    string   `json:"file"`
	Line    int      `json:"line"`
	Path    []string `json:"path"`
	Status  string   `json:"status"`
}

func readDoc(t *gotest.T, run *testkit.Run) document {
	var doc document
	gotest.NoError(t, json.Unmarshal([]byte(run.Stdout), &doc), "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
	return doc
}

// byTitle is the document's scenario of that title in that file.
func byTitle(t *gotest.T, doc *document, file, title string) scenario {
	for i := range doc.Scenarios {
		if sc := &doc.Scenarios[i]; sc.File == file && sc.Title == title {
			return *sc
		}
	}
	gotest.Fail(t, "no scenario "+title+" in "+file)
	return scenario{}
}

// statuses maps each scenario of a file to its status.
func statuses(doc *document, file string) map[string]string {
	out := map[string]string{}
	for i := range doc.Scenarios {
		if sc := &doc.Scenarios[i]; sc.File == file {
			out[sc.Title] = sc.Status
		}
	}
	return out
}

// at is a place as file:line.
func at(ps []place) []string {
	out := []string{}
	for _, p := range ps {
		out = append(out, p.File+":"+strconv.Itoa(p.Line))
	}
	return out
}

// tempDir is a fresh directory for one suite's staged trees.
func tempDir(t *gotest.T, pattern string) string {
	dir, err := os.MkdirTemp("", pattern)
	gotest.NoError(t, err)
	return dir
}
