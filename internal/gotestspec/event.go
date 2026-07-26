package gotestspec

import (
	"bufio"
	"encoding/json"
	"io"
	"time"
)

type Action string

const (
	ActionRun    Action = "run"
	ActionOutput Action = "output"
	ActionPass   Action = "pass"
	ActionFail   Action = "fail"
	ActionSkip   Action = "skip"
	// ActionBench is not emitted by go test's own -json encoder (real
	// benchmark runs only ever produce "run"/"output", plus "fail" on
	// failure — there is no terminal "pass"-equivalent event for a
	// benchmark). It is accepted here so BuildTree can also finalize a
	// benchmark node's status when fed synthetic or hand-authored event
	// streams (e.g. via `gotest spec --input`) that choose to emit one.
	ActionBench Action = "bench"
)

type TestEvent struct {
	Time    time.Time `json:"Time"`
	Action  Action    `json:"Action"`
	Package string    `json:"Package"`
	Test    string    `json:"Test"`
	Output  string    `json:"Output"`
	Elapsed float64   `json:"Elapsed"`
}

func ParseEvents(r io.Reader) ([]TestEvent, error) {
	var events []TestEvent
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var ev TestEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		events = append(events, ev)
	}
	return events, scanner.Err()
}
