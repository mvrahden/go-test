// Package sharedwire is the conversation between the runner and the shared
// fixture setup program, defined once for both ends.
//
// The runner writes commands to the program's stdin, one JSON object a line:
// start or tear down fixtures by state key (each with an id), stop, force.
// Stdin ending means the runner is gone.
//
// The program writes lines to its protocol stream (descriptor 3, its stdout
// on Windows), one JSON object a line keyed by "key": a fixture's state under
// its state key, and the sentinels, whose keys start with "_". Per fixture,
// _setup comes before its state; _done ends the up-front setup; _cmd
// acknowledges a command by its id; _teardown names the teardowns that
// failed, before the program exits.
//
// A state file holds each fixture's state under its state key.
//
// Lines and fields are only ever added: a reader ignores a sentinel or a
// field it does not know.
package sharedwire

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Verbs of a command.
const (
	VerbStart    = "start"
	VerbTeardown = "teardown"
	VerbStop     = "stop"
	VerbForce    = "force"
)

// Keys of the lines that are not a fixture's state.
const (
	KeySetup    = "_setup"
	KeyDone     = "_done"
	KeyCmd      = "_cmd"
	KeyTeardown = "_teardown"
)

// ExitLifecycleFailed is the process's status when a setup or a teardown failed.
const ExitLifecycleFailed = 1

// SetupFailed is the error of a _done line after a failed setup.
const SetupFailed = "one or more shared fixtures failed"

// ReleaseRefused ends the error of a release refused whole; the runner reads it.
const ReleaseRefused = "is up and not released with it"

// Command is one line the runner writes to the process's stdin.
type Command struct {
	Verb string   `json:"verb"`
	Keys []string `json:"keys,omitempty"`
	ID   uint64   `json:"id,omitempty"`
}

// Line is the command as one line, without its newline.
func (c Command) Line() []byte {
	b, _ := json.Marshal(c) // strings and a number: cannot fail
	return b
}

// ParseCommand reads one command line; false for a line that is none.
func ParseCommand(line []byte) (Command, bool) {
	var c Command
	if json.Unmarshal(line, &c) != nil {
		return Command{}, false
	}
	return c, true
}

// Line is one line the process writes to its protocol stream, decoded.
// Only for reading: the encoders below write each kind's exact fields.
type Line struct {
	Key            string          `json:"key"`
	Fixture        string          `json:"fixture,omitempty"`
	State          json.RawMessage `json:"state,omitempty"`
	Budget         string          `json:"budget,omitempty"`
	TeardownBudget string          `json:"teardownBudget,omitempty"`
	Error          string          `json:"error,omitempty"`
	ID             uint64          `json:"id,omitempty"`
}

// ParseLine reads one protocol line; false for a line that is not JSON.
func ParseLine(line []byte) (Line, bool) {
	var l Line
	if json.Unmarshal(line, &l) != nil {
		return Line{}, false
	}
	return l, true
}

// Sentinel reports whether l is not a fixture's state: its key starts with "_".
func (l Line) Sentinel() bool { return strings.HasPrefix(l.Key, "_") } //nolint:gocritic // hugeParam: a decoded line

// quote is s as a JSON string; <, > and & are escaped.
func quote(s string) string {
	b, _ := json.Marshal(s) // a string: cannot fail
	return string(b)
}

// SetupLine says the fixture's setup begins, held to budget.
func SetupLine(fixtureKey string, budget time.Duration) []byte {
	return []byte(`{"key":"` + KeySetup + `","fixture":` + quote(fixtureKey) + `,"budget":` + quote(budget.String()) + `}`)
}

// StateLine carries a fixture's state, JSON already; nil or empty is {}.
func StateLine(fixtureKey string, state []byte) []byte {
	if len(state) == 0 {
		state = []byte("{}")
	}
	return []byte(`{"key":` + quote(fixtureKey) + `,"state":` + string(state) + `}`)
}

// DoneLine ends the up-front setup; setupErr is empty for a good one.
func DoneLine(setupErr string, teardownBudget time.Duration) []byte {
	errField := ""
	if setupErr != "" {
		errField = `"error":` + quote(setupErr) + `,`
	}
	return []byte(`{"key":"` + KeyDone + `",` + errField + `"teardownBudget":` + quote(teardownBudget.String()) + `}`)
}

// AckLine acknowledges command id; err is always written, empty for none.
func AckLine(id uint64, err string, teardownBudget time.Duration) []byte {
	return []byte(`{"key":"` + KeyCmd + `","id":` + strconv.FormatUint(id, 10) + `,"error":` + quote(err) + `,"teardownBudget":` + quote(teardownBudget.String()) + `}`)
}

// TeardownLine names the teardowns that failed.
func TeardownLine(err string) []byte {
	return []byte(`{"key":"` + KeyTeardown + `","error":` + quote(err) + `}`)
}

// State is a state file: each fixture's state under its key.
type State map[string]json.RawMessage

// Encode is the state file's content.
func (s State) Encode() ([]byte, error) { return json.Marshal(s) }

// DecodeState reads a state file's content.
func DecodeState(data []byte) (State, error) {
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("unmarshal shared state: %w", err)
	}
	return s, nil
}
