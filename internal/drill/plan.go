// Package drill runs the mutation drill: it plans, schedules and supervises
// one worker per mutant and accounts for their verdicts. Only a worker
// applies a mutation, to a copied source tree; everything else runs pristine.
package drill

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/mvrahden/go-test/internal/protocol"
)

// SharedDivisor is each shared mutant's share of a shard's wall time. Fixed,
// so shards whose runners differ in CPUs still compute one plan and split it
// exactly.
const SharedDivisor = 2

// defaultWeight is the estimate for a mutant without a recorded duration.
const defaultWeight = 30

// Run is one judge invocation: a -run filter ("-" for none) and packages.
// It reads and writes as [filter, [packages...]].
type Run struct {
	Pattern  string
	Packages []string
}

func (r Run) MarshalJSON() ([]byte, error) {
	pkgs := r.Packages
	if pkgs == nil {
		pkgs = []string{}
	}
	return json.Marshal([]any{r.Pattern, pkgs})
}

func (r *Run) UnmarshalJSON(data []byte) error {
	var fields []json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if len(fields) != 2 {
		return errors.New("a run is [filter, [packages]]")
	}
	if err := json.Unmarshal(fields[0], &r.Pattern); err != nil {
		return err
	}
	return json.Unmarshal(fields[1], &r.Packages)
}

func weight(weights map[string]float64, name string) float64 {
	if w, ok := weights[name]; ok {
		return w
	}
	return defaultWeight
}

// Schedule assigns names to shards longest first, each to the shard with the
// least projected wall time; deterministic across shards.
func Schedule(names []string, modes map[string]string, weights map[string]float64, shards int) [][]string {
	buckets := make([][]string, shards)
	for i := range buckets {
		buckets[i] = []string{}
	}
	load := make([]float64, shards)
	order := slices.Clone(names)
	slices.SortStableFunc(order, func(a, b string) int {
		if wa, wb := weight(weights, a), weight(weights, b); wa != wb {
			if wa > wb {
				return -1
			}
			return 1
		}
		return strings.Compare(a, b)
	})
	for _, name := range order {
		slot := 0
		for i := 1; i < shards; i++ {
			if load[i] < load[slot] {
				slot = i
			}
		}
		buckets[slot] = append(buckets[slot], name)
		w := weight(weights, name)
		if modes[name] == "shared" {
			w /= SharedDivisor
		}
		load[slot] += w
	}
	return buckets
}

// Plan returns every shard's mutants and this shard's concurrent worker
// count. The assignment reads only what every shard shares; the local CPU
// count and DRILL_JOBS set the workers alone.
func Plan(names []string, modes map[string]string, weights map[string]float64, shards int, environ map[string]string, cpus int) ([][]string, int, error) {
	jobs, err := WorkerCount(environ, cpus)
	return Schedule(names, modes, weights, shards), jobs, err
}

// WorkerCount is DRILL_JOBS, or half the CPUs and at least one.
func WorkerCount(environ map[string]string, cpus int) (int, error) {
	value, ok := environ["DRILL_JOBS"]
	if !ok {
		value = strconv.Itoa(max(1, cpus/2))
	}
	return Positive(value, "DRILL_JOBS")
}

// Positive parses a positive integer, naming label when it is not one.
func Positive(value, label string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || n < 1 {
		return 0, errors.New(label + " must be a positive integer")
	}
	return n, nil
}

// Selection is one shard's selection-i-of-n.json.
type Selection struct {
	Shard      int             `json:"shard"`
	Shards     int             `json:"shards"`
	Assignment [][]string      `json:"assignment"`
	Mutants    SelectedMutants `json:"mutants"`
}

// SelectedMutant is what a shard records about one of its mutants.
type SelectedMutant struct {
	Mode             string  `json:"mode"`
	Runs             []Run   `json:"runs"`
	EstimatedSeconds float64 `json:"estimated_seconds"`
}

// SelectedMutants keeps the shard's own order when written.
type SelectedMutants struct {
	Names  []string
	ByName map[string]SelectedMutant
}

func (m SelectedMutants) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, name := range m.Names {
		if i > 0 {
			b.WriteByte(',')
		}
		key, _ := json.Marshal(name)
		value, err := json.Marshal(m.ByName[name])
		if err != nil {
			return nil, err
		}
		b.Write(key)
		b.WriteByte(':')
		b.Write(value)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

func (m *SelectedMutants) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if raw == nil {
		return errors.New("mutants is not an object")
	}
	m.ByName = map[string]SelectedMutant{}
	m.Names = m.Names[:0]
	for name := range raw {
		m.Names = append(m.Names, name)
	}
	slices.Sort(m.Names)
	return nil
}

// WriteJSON writes v indented by two spaces, as the drill's JSON files are.
func WriteJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

// FindSelections lists the selection files under dir, in path order.
func FindSelections(dir string) []string {
	var found []string
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if ok, _ := filepath.Match("selection-*-of-*.json", d.Name()); ok && path != dir {
			found = append(found, path)
		}
		return nil
	})
	return found
}

// CheckShards names what is wrong with the selection files of one sharded
// run; none means the shards split the mutants exactly.
func CheckShards(paths []string) []string {
	selections := make([]Selection, 0, len(paths))
	for _, path := range paths {
		s, err := readSelection(path)
		if err != nil {
			return []string{"unreadable selection file"}
		}
		selections = append(selections, s)
	}
	if len(selections) == 0 {
		return []string{"no selection files"}
	}
	var problems []string
	first := selections[0]
	for _, s := range selections {
		if s.Shards != first.Shards || !equalAssignments(s.Assignment, first.Assignment) {
			problems = append(problems, "shards computed different assignments")
			break
		}
	}
	shards := make([]int, 0, len(selections))
	for _, s := range selections {
		shards = append(shards, s.Shard)
	}
	slices.Sort(shards)
	want := make([]int, 0, max(first.Shards, 0))
	for i := 1; i <= first.Shards; i++ {
		want = append(want, i)
	}
	if !slices.Equal(shards, want) {
		problems = append(problems, fmt.Sprintf("expected one selection per shard 1..%d, got %s", first.Shards, intList(shards)))
	}
	for _, s := range selections {
		if s.Shard < 1 || s.Shard > len(s.Assignment) || !sameMembers(s.Mutants.Names, s.Assignment[s.Shard-1]) {
			problems = append(problems, fmt.Sprintf("shard %d selected other mutants than its assignment names", s.Shard))
		}
	}
	return problems
}

func readSelection(path string) (Selection, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Selection{}, err
	}
	var present map[string]json.RawMessage
	if err := json.Unmarshal(data, &present); err != nil {
		return Selection{}, err
	}
	for _, key := range []string{"shard", "shards", "assignment", "mutants"} {
		if _, ok := present[key]; !ok {
			return Selection{}, errors.New("selection lacks " + key)
		}
	}
	var s Selection
	err = json.Unmarshal(data, &s)
	return s, err
}

func equalAssignments(a, b [][]string) bool {
	return slices.EqualFunc(a, b, slices.Equal[[]string])
}

func sameMembers(a, b []string) bool {
	a, b = slices.Clone(a), slices.Clone(b)
	slices.Sort(a)
	slices.Sort(b)
	return slices.Equal(a, b)
}

func intList(ns []int) string {
	parts := make([]string, len(ns))
	for i, n := range ns {
		parts[i] = strconv.Itoa(n)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// lines splits text into lines as a reader of line-oriented output does:
// no trailing empty line, and a CR before a newline dropped.
func lines(text string) []string {
	if text == "" {
		return nil
	}
	out := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	for i, line := range out {
		out[i] = strings.TrimSuffix(line, "\r")
	}
	return out
}

// ReadPlan reads the planner's mutant and run lines for exactly names.
func ReadPlan(text string, names []string) (map[string]string, map[string][]Run, error) {
	modes, runs := map[string]string{}, map[string][]Run{}
	for _, name := range names {
		runs[name] = []Run{}
	}
	for _, line := range lines(text) {
		fields := strings.Split(line, "\t")
		switch _, known := runs[get(fields, 1)]; {
		case len(fields) == 3 && fields[0] == "mutant" && known:
			name, mode := fields[1], fields[2]
			if _, dup := modes[name]; dup || (mode != "alone" && mode != "shared") {
				return nil, nil, errors.New("duplicate or invalid mutant: " + name)
			}
			modes[name] = mode
		case len(fields) == 4 && fields[0] == "run" && known:
			runs[fields[1]] = append(runs[fields[1]], Run{Pattern: fields[2], Packages: strings.Fields(fields[3])})
		default:
			return nil, nil, errors.New("invalid judge plan: " + line)
		}
	}
	complete := len(modes) == len(runs)
	for name, r := range runs {
		if _, ok := modes[name]; !ok || len(r) == 0 {
			complete = false
		}
	}
	if !complete {
		return nil, nil, errors.New("incomplete judge plan")
	}
	return modes, runs, nil
}

func get(fields []string, i int) string {
	if i < len(fields) {
		return fields[i]
	}
	return ""
}

// BuildFailure reports whether a judge's -json stream carries a build-fail
// event; a line that is not an event is an error.
func BuildFailure(stream string) (bool, error) {
	for _, line := range lines(stream) {
		var event any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return false, errors.New("judge emitted a non-JSON line")
		}
		fields, ok := event.(map[string]any)
		action, isString := fields["Action"].(string)
		if !ok || !isString {
			return false, errors.New("judge emitted an invalid event")
		}
		if action == "build-fail" {
			return true, nil
		}
	}
	return false, nil
}

// Status is what a supervisor observed of its command.
type Status struct {
	Exit         *int    `json:"exit"`
	Timeout      bool    `json:"timeout"`
	Interrupted  bool    `json:"interrupted"`
	CleanupError string  `json:"cleanup_error,omitempty"`
	Seconds      float64 `json:"seconds"`
}

// Verdict is one mutant's terminal result.
type Verdict struct {
	Name       string             `json:"name"`
	Caught     bool               `json:"caught"`
	Reason     string             `json:"reason"`
	Seconds    float64            `json:"seconds"`
	Phases     map[string]float64 `json:"phases,omitempty"`
	Supervisor *Status            `json:"supervisor,omitempty"`
}

// ReadResult is the verdict for name: the worker's result file when the
// worker and its supervisor ended cleanly, otherwise a failure naming why.
// supervisor is nil when its status could not be read.
func ReadResult(path, name string, supervisor *Status, supervisorExit int) Verdict {
	s := supervisor
	if s == nil {
		s = &Status{}
	}
	if supervisorExit != 0 || s.Exit == nil || *s.Exit != 0 || s.Timeout || s.Interrupted || s.CleanupError != "" {
		reason := "worker or supervisor exited without a valid result"
		if s.Exit != nil && *s.Exit < 0 {
			reason = "worker ended by " + signalName(-*s.Exit)
		}
		switch {
		case s.Timeout:
			reason = "independent watchdog deadline exceeded"
		case s.Interrupted:
			reason = "worker cancelled"
		case s.CleanupError != "":
			reason = "process cleanup failed: " + s.CleanupError
		}
		return Verdict{Name: name, Reason: reason, Seconds: s.Seconds, Supervisor: s}
	}
	invalid := Verdict{Name: name, Reason: "worker produced no valid terminal result"}
	data, err := os.ReadFile(path)
	if err != nil {
		return invalid
	}
	var fields map[string]any
	if json.Unmarshal(data, &fields) != nil || fields["name"] != name {
		return invalid
	}
	if _, ok := fields["caught"].(bool); !ok {
		return invalid
	}
	var v Verdict
	if json.Unmarshal(data, &v) != nil {
		return invalid
	}
	return v
}

// hostOnly are variables that change what the checks under judgment do:
// CI=true makes the CLI set GOTEST_CI, under which a missing snapshot
// baseline fails on its own.
var hostOnly = []string{"CI", "GITHUB_ACTIONS", "GITHUB_STEP_SUMMARY"}

// WorkerEnv is the judge's environment, the same on a CI runner as on a laptop.
// The slow-build threshold passes: it describes the machine, which a slow
// runner makes slow for the judge's builds too, not the checks.
func WorkerEnv(base map[string]string, work, tmp string, cpus int) map[string]string {
	env := map[string]string{}
	for k, v := range base {
		gotestVar := strings.HasPrefix(k, "GOTEST_") && k != protocol.EnvSlowBuildThreshold
		if !slices.Contains(hostOnly, k) && !gotestVar {
			env[k] = v
		}
	}
	env["TMPDIR"] = tmp
	env["GOTEST_CACHE_DIR"] = filepath.Join(work, "cache", "judge")
	env["GOTEST_TEST_CACHE_DIR"] = filepath.Join(work, "cache", "children")
	env["GOTEST_TEST_CLI_CACHE_DIR"] = filepath.Join(work, "cache", "cli")
	env["GOMAXPROCS"] = strconv.Itoa(cpus)
	env["GOFLAGS"] = env["GOFLAGS"] + " -p=" + strconv.Itoa(cpus)
	return env
}

// Environ is the process environment as a map.
func Environ() map[string]string {
	env := map[string]string{}
	for _, kv := range os.Environ() {
		if k, v, ok := strings.Cut(kv, "="); ok {
			env[k] = v
		}
	}
	return env
}

func envList(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	slices.Sort(out)
	return out
}

// WriteJSONLine writes v as one compact line.
func WriteJSONLine(path string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}
