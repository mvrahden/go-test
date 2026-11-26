package gotestspec

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"time"

	"github.com/mvrahden/go-test/internal/protocol"
)

type Action string

const (
	ActionRun    Action = "run"
	ActionOutput Action = "output"
	ActionPass   Action = "pass"
	ActionFail   Action = "fail"
	ActionSkip   Action = "skip"
	// A test that calls t.Parallel is parked and resumed, which is the one
	// thing that makes its own timestamps untrustworthy — see Node.Paused.
	ActionPause Action = "pause"
	ActionCont  Action = "cont"
	// ActionBench is not emitted by go test's own -json encoder (real
	// benchmark runs only ever produce "run"/"output", plus "fail" on
	// failure — there is no terminal "pass"-equivalent event for a
	// benchmark). It is accepted here so BuildTree can also finalize a
	// benchmark node's status when fed synthetic or hand-authored event
	// streams (e.g. via `gotest spec --input`) that choose to emit one.
	ActionBench Action = "bench"
	// go test (Go 1.24+) reports a build as events of its own, keyed by the
	// ImportPath of what was built; the package that build failed names it
	// in its verdict's FailedBuild.
	ActionBuildOutput Action = "build-output"
	ActionBuildFail   Action = "build-fail"
)

type TestEvent struct {
	Time    time.Time `json:"Time"`
	Action  Action    `json:"Action"`
	Package string    `json:"Package"`
	Test    string    `json:"Test"`
	Output  string    `json:"Output"`
	Elapsed float64   `json:"Elapsed"`
	// ImportPath names the build a build-output or build-fail event is about.
	ImportPath string `json:"ImportPath"`
	// FailedBuild names the build whose failure failed this package.
	FailedBuild string `json:"FailedBuild"`
	// RunLevel marks an event of a package gotest's run booked for itself
	// ("shared fixtures", "interrupted", …) rather than for an import path.
	RunLevel bool `json:"RunLevel"`
	// Gotest says where the test sits, when gotest's runner wrote the event.
	Gotest *protocol.Stamp `json:"Gotest,omitempty"`
	// Manifest and Result are the run's own: its start says what it was,
	// its verdict what it judged (protocol.RunPackage).
	Manifest *protocol.Manifest  `json:"Manifest,omitempty"`
	Result   *protocol.RunResult `json:"Result,omitempty"`
}

// ParseEvents reads one event per line, a line of any length; blank and
// malformed lines are skipped. Only a read error is returned.
func ParseEvents(r io.Reader) ([]TestEvent, error) {
	var events []TestEvent
	br := bufio.NewReaderSize(r, 64*1024)
	for {
		line, err := br.ReadBytes('\n')
		if err != nil && err != io.EOF {
			return events, err
		}
		line = bytes.TrimRight(line, "\r\n")
		if len(line) > 0 {
			var ev TestEvent
			if json.Unmarshal(line, &ev) == nil {
				events = append(events, ev)
			}
		}
		if err == io.EOF {
			return events, nil
		}
	}
}
