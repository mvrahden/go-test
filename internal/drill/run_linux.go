package drill

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

var mutantName = regexp.MustCompile(`^[a-z0-9-]+$`)

func superviseCommand(seconds float64, status string, command []string) (int, error) {
	return supervise(seconds, status, command), nil
}

// runDrill judges the named mutants, or all of them, of the repository at
// root, and returns 0 only when every selected mutant was caught.
func runDrill(root string, names []string) (int, error) {
	if root == "" {
		return 0, errors.New("no --root; run the drill through tests/drill/drill.sh")
	}
	if err := EnableSubreaper(); err != nil {
		return 0, err
	}
	environ := Environ()
	cpus := availableCPUs()
	jobs, err := WorkerCount(environ, cpus)
	if err != nil {
		return 0, err
	}
	shard := environ["DRILL_SHARD"]
	if _, ok := environ["DRILL_SHARD"]; !ok {
		shard = "1/1"
	}
	parts := strings.Split(shard, "/")
	if len(parts) != 2 {
		return 0, errors.New("DRILL_SHARD must be i/n with 1 <= i <= n")
	}
	shardI, err := Positive(parts[0], "DRILL_SHARD")
	if err != nil {
		return 0, err
	}
	shardN, err := Positive(parts[1], "DRILL_SHARD")
	if err != nil {
		return 0, err
	}
	if shardI > shardN {
		return 0, errors.New("DRILL_SHARD must be i/n with 1 <= i <= n")
	}
	timeout := environ["DRILL_TIMEOUT"]
	if _, ok := environ["DRILL_TIMEOUT"]; !ok {
		timeout = "600"
	}
	seconds, err := Positive(timeout, "DRILL_TIMEOUT")
	if err != nil {
		return 0, err
	}
	mutants := filepath.Join(root, "tests", "drill", "mutants")
	if len(names) == 0 {
		patches, _ := filepath.Glob(filepath.Join(mutants, "*.patch"))
		for _, p := range patches {
			names = append(names, strings.TrimSuffix(filepath.Base(p), ".patch"))
		}
		slices.Sort(names)
	}
	seen := map[string]bool{}
	for _, name := range names {
		if seen[name] {
			return 0, errors.New("a mutant was selected more than once")
		}
		seen[name] = true
	}
	for _, name := range names {
		info, err := os.Stat(filepath.Join(mutants, name+".patch"))
		if !mutantName.MatchString(name) || err != nil || !info.Mode().IsRegular() {
			return 0, errors.New("no mutant named " + name)
		}
	}
	weights := map[string]float64{}
	if path := environ["DRILL_TIMINGS"]; path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return 0, err
		}
		var raw map[string]any
		if err := json.Unmarshal(data, &raw); err != nil || raw == nil {
			return 0, errors.New("DRILL_TIMINGS must map mutant names to positive finite seconds")
		}
		for name, v := range raw {
			w, ok := v.(float64)
			if !ok || math.IsInf(w, 0) || math.IsNaN(w) || w <= 0 {
				return 0, errors.New("DRILL_TIMINGS must map mutant names to positive finite seconds")
			}
			weights[name] = w
		}
	}
	started := time.Now()
	logRoot := environ["DRILL_LOGS"]
	if logRoot != "" {
		if err := os.MkdirAll(logRoot, 0o777); err != nil {
			return 0, err
		}
	}
	// Concurrent invocations, including shards on one machine, never share
	// status files or mistake another invocation's verdict for their own.
	logs, err := os.MkdirTemp(logRoot, fmt.Sprintf("drill-%d-of-%d-", shardI, shardN))
	if err != nil {
		return 0, err
	}
	if logs, err = realPath(logs); err != nil {
		return 0, err
	}
	fmt.Println("drill  logs: " + logs)
	self, err := os.Executable()
	if err != nil {
		return 0, err
	}
	d := &drillRun{
		root: root, logs: logs, logRoot: logRoot, names: names, weights: weights,
		shardI: shardI, shardN: shardN, cpus: cpus, jobs: jobs, self: self,
		supervision: NewSupervision([]string{self}, float64(seconds), logs),
		results:     map[string]Verdict{},
	}
	var runErr error
	WatchSignals(d.supervision, func() { runErr = d.judge() })
	timings := map[string]float64{}
	for name, r := range d.results {
		if r.Caught && r.Seconds > 0 {
			timings[name] = r.Seconds
		}
	}
	if err := WriteJSON(filepath.Join(logs, fmt.Sprintf("timings-%d-of-%d.json", shardI, shardN)), timings); err != nil {
		return 0, err
	}
	if runErr != nil {
		return 0, runErr
	}
	var missing []string
	for _, name := range d.names {
		if _, ok := d.results[name]; !ok {
			missing = append(missing, name)
		}
	}
	slices.Sort(missing)
	for _, name := range missing {
		fmt.Println("DRILL  " + name + ": no terminal result")
	}
	fmt.Printf("drill  %d/%d mutants judged in %.1fs\n", len(d.results), len(d.names), time.Since(started).Seconds())
	failed := d.supervision.Stopping() || len(missing) > 0
	for _, r := range d.results {
		failed = failed || !r.Caught
	}
	if failed {
		return 1, nil
	}
	return 0, nil
}

func realPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}

type drillRun struct {
	root, logs, logRoot, self  string
	names                      []string
	weights                    map[string]float64
	shardI, shardN, cpus, jobs int
	supervision                *Supervision
	results                    map[string]Verdict
}

// judge snapshots the repository, bootstraps the pristine judge and planner
// from the snapshot, and judges this shard's mutants.
func (d *drillRun) judge() (err error) {
	scratch, err := os.MkdirTemp("", "gotest-drill-")
	if err != nil {
		return err
	}
	defer func() {
		if rerr := removeAll(scratch); rerr != nil && err == nil {
			err = rerr
		}
	}()
	// If a supervisor itself dies, its detached descendants are adopted here
	// and reaped before the source and cache directories are removed.
	defer func() {
		if cerr := Clean(nil); cerr != nil {
			err = cerr
		}
	}()
	snapshot := filepath.Join(scratch, "snapshot")
	logRoot := ""
	if d.logRoot != "" {
		logRoot, _ = realPath(d.logRoot)
	}
	skip := func(path string, e fs.DirEntry) bool {
		switch e.Name() {
		case ".git", ".worktrees", "node_modules":
			return true
		}
		if !e.IsDir() && e.Type()&fs.ModeSymlink == 0 {
			return false
		}
		resolved, err := realPath(path)
		return err == nil && (resolved == d.logs || (logRoot != "" && resolved == logRoot))
	}
	if err := copyTree(d.root, snapshot, skip); err != nil {
		return err
	}
	judge, clauses := filepath.Join(scratch, "gotest"), filepath.Join(scratch, "clauses")
	patchOf := func(name string) string {
		return filepath.Join(snapshot, "tests", "drill", "mutants", name+".patch")
	}
	// Bootstrap and planning use the same independent deadline and
	// cancellation cleanup as mutants; partial plans never schedule.
	output, err := os.Create(filepath.Join(d.logs, "bootstrap.log"))
	if err != nil {
		return err
	}
	defer func() { _ = output.Close() }()
	ok := func(code int, observed *Status, err error) (bool, error) {
		return err == nil && code == 0 && observed != nil && observed.Exit != nil && *observed.Exit == 0, err
	}
	for _, b := range []struct{ binary, pkg string }{{judge, "./cmd/gotest"}, {clauses, "./internal/speccatalog/clauses"}} {
		name := filepath.Base(b.binary)
		if fine, err := ok(d.supervision.Run([]string{"go", "build", "-o", b.binary, b.pkg}, name+"-build", output, output, snapshot)); err != nil {
			return err
		} else if !fine {
			return errors.New("could not build " + name + "; see bootstrap.log")
		}
	}
	// Go writes a tool into the shared build cache in place; a run that
	// resolves it meanwhile fails to exec it (text file busy).
	if fine, err := ok(d.supervision.Run([]string{"go", "tool", "-n", "test2json"}, "test2json-build", output, output, snapshot)); err != nil {
		return err
	} else if !fine {
		return errors.New("could not build test2json; see bootstrap.log")
	}
	planPath := filepath.Join(d.logs, "plan.tsv")
	planOut, err := os.Create(planPath)
	if err != nil {
		return err
	}
	command := []string{clauses, "--judge-runs"}
	for _, n := range d.names {
		command = append(command, patchOf(n))
	}
	fine, err := ok(d.supervision.Run(command, "plan", planOut, output, snapshot))
	if cerr := planOut.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if !fine {
		return errors.New("could not read judge plan; see plan.tsv and bootstrap.log")
	}
	text, err := os.ReadFile(planPath)
	if err != nil {
		return err
	}
	modes, runs, err := ReadPlan(string(text), d.names)
	if err != nil {
		return err
	}
	assignment, _, err := Plan(d.names, modes, d.weights, d.shardN, Environ(), d.cpus)
	if err != nil {
		return err
	}
	d.names = assignment[d.shardI-1]
	// The whole assignment, so --check-shards can prove the shards agree.
	selected := SelectedMutants{Names: d.names, ByName: map[string]SelectedMutant{}}
	for _, n := range d.names {
		selected.ByName[n] = SelectedMutant{Mode: modes[n], Runs: runs[n], EstimatedSeconds: weight(d.weights, n)}
	}
	if err := WriteJSON(filepath.Join(d.logs, fmt.Sprintf("selection-%d-of-%d.json", d.shardI, d.shardN)),
		Selection{Shard: d.shardI, Shards: d.shardN, Assignment: assignment, Mutants: selected}); err != nil {
		return err
	}
	execute := func(worker int, name string, cpus int) (Verdict, *Status, error) {
		work := filepath.Join(scratch, "w"+strconv.Itoa(worker))
		if err := os.Mkdir(work, 0o777); err != nil && !errors.Is(err, fs.ErrExist) {
			return Verdict{}, nil, err
		}
		result := filepath.Join(d.logs, name+".result.json")
		// A previous run's result must never count for this one.
		if err := os.Remove(result); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return Verdict{}, nil, err
		}
		spec := Spec{Name: name, Work: work, Snapshot: snapshot, Patch: patchOf(name),
			Log: filepath.Join(d.logs, name+".log"), Result: result, Runs: runs[name], Judge: judge, CPUs: cpus}
		config := filepath.Join(work, "job.json")
		if err := WriteJSONLine(config, spec); err != nil {
			return Verdict{}, nil, err
		}
		out, err := os.Create(filepath.Join(d.logs, name+".worker.log"))
		if err != nil {
			return Verdict{}, nil, err
		}
		code, observed, err := d.supervision.Run([]string{d.self, "--worker", config}, name, out, out, "")
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return Verdict{}, nil, err
		}
		verdict := ReadResult(result, name, observed, code)
		if err := WriteJSONLine(result, verdict); err != nil {
			return Verdict{}, nil, err
		}
		return verdict, observed, nil
	}
	var shared, alone []string
	for _, n := range d.names {
		if modes[n] == "shared" {
			shared = append(shared, n)
		} else {
			alone = append(alone, n)
		}
	}
	return JudgeAll([]Phase{{shared, d.jobs, max(1, d.cpus/d.jobs)}, {alone, 1, d.cpus}}, d.supervision, execute, d.results)
}
