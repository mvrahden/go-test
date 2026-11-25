package gotestrunner

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/mvrahden/go-test/internal/protocol"
)

// failedUnit is the unit --failed reruns for a failed test of pkg: the
// method of a deeper subtest, the wrapper of a fuzz seed, the package itself
// for no test.
func failedUnit(pkg, test string) protocol.FailedUnit {
	levels := strings.SplitN(test, "/", 3)
	switch {
	case test == "":
	case strings.HasPrefix(levels[0], protocol.PrefixFuzz) || len(levels) == 1:
		test = levels[0]
	default:
		test = levels[0] + "/" + levels[1]
	}
	return protocol.FailedUnit{Package: pkg, Test: test}
}

// narrowFailures keeps the finest units of failed, in order: a failed method
// stands for its suite, and any failed unit for its package.
func narrowFailures(failed []protocol.FailedUnit) []protocol.FailedUnit {
	finer := map[protocol.FailedUnit]bool{}
	for _, u := range failed {
		if u.Test == "" {
			continue
		}
		finer[protocol.FailedUnit{Package: u.Package}] = true
		if suite, _, ok := strings.Cut(u.Test, "/"); ok {
			finer[protocol.FailedUnit{Package: u.Package, Test: suite}] = true
		}
	}
	var out []protocol.FailedUnit
	for _, u := range failed {
		if !finer[u] && !slices.Contains(out, u) {
			out = append(out, u)
		}
	}
	return out
}

// textFailures are the units a failed suite of a text run names in its
// result lines, the suite itself among them; a result without a suite is its
// package's, as a build failure is.
func textFailures(pkg string, r *SuiteResult) []protocol.FailedUnit {
	if r.Target.SuiteName == "" {
		return []protocol.FailedUnit{{Package: pkg}}
	}
	out := []protocol.FailedUnit{{Package: pkg, Test: r.Target.SuiteName}}
	for line := range bytes.SplitSeq(r.Stdout, []byte("\n")) {
		rest, ok := strings.CutPrefix(strings.TrimLeft(string(line), " \t"), "--- FAIL: ")
		if !ok {
			continue
		}
		if at := strings.LastIndex(rest, " ("); at > 0 {
			out = append(out, failedUnit(pkg, rest[:at]))
		}
	}
	return out
}

// judged is what the collector's run judged and the units it failed, read
// once the run has booked everything; nil for a stream without a tally.
func (c *OutputCollector) judged() (*protocol.RunResult, []protocol.FailedUnit) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.mode != RunBatchText {
		if c.starts == nil || c.starts.run == nil {
			return nil, nil
		}
		res, failed := c.starts.run.snapshot(c.runLevel)
		return &res, narrowFailures(append(failed, c.vacuousUnits()...))
	}
	res := protocol.RunResult{NotRun: c.notRun, Vacuous: c.vacuous}
	var failed []protocol.FailedUnit
	missed := map[string]bool{}
	for _, m := range c.missing {
		missed[m.Pkg] = true
	}
	for _, pkg := range c.order {
		s := c.pkgs[pkg]
		bad := s.failedBuild != ""
		for i := range s.results {
			if s.results[i].ExitCode != 0 {
				bad = true
				failed = append(failed, textFailures(pkg, &s.results[i])...)
			}
		}
		switch {
		case bad || missed[pkg]:
			res.Packages.Add("fail")
		case strings.HasPrefix(s.fixed, "?"):
			res.Packages.Add("skip")
		case s.fixed != "" || s.completed > 0:
			res.Packages.Add("pass")
		}
	}
	for _, m := range c.missing {
		failed = append(failed, failedUnit(m.Pkg, m.Path))
	}
	return &res, narrowFailures(append(failed, c.vacuousUnits()...))
}

// vacuousUnits are the units of the passes without an assertion that failed
// the run under --ci.
func (c *OutputCollector) vacuousUnits() []protocol.FailedUnit {
	var out []protocol.FailedUnit
	for _, v := range c.vacuousFailed {
		out = append(out, failedUnit(v.Pkg, v.Path))
	}
	return out
}

// RerunSelection is the per-package -run that reruns failed, as
// PipelineConfig.Only takes it: "" for a package that failed whole.
func RerunSelection(failed []protocol.FailedUnit) map[string]string {
	alts := map[string][]string{}
	whole := map[string]bool{}
	for _, u := range failed {
		if u.Test == "" {
			whole[u.Package] = true
			continue
		}
		levels := strings.Split(u.Test, "/")
		for i := range levels {
			levels[i] = "^" + regexp.QuoteMeta(levels[i]) + "$"
		}
		alts[u.Package] = append(alts[u.Package], strings.Join(levels, "/"))
	}
	only := map[string]string{}
	for pkg, a := range alts {
		only[pkg] = strings.Join(a, "|")
	}
	for pkg := range whole {
		only[pkg] = ""
	}
	return only
}

// RunRecordPath is where the runs made in dir leave their record: under the
// gotest cache, keyed by the directory, never in the module itself.
func RunRecordPath(dir string) (string, error) {
	root, err := cacheRoot()
	if err != nil {
		return "", err
	}
	return recordPathIn(root, dir), nil
}

func recordPathIn(root, dir string) string {
	sum := sha256.Sum256([]byte(dir))
	return filepath.Join(root, "runs", hex.EncodeToString(sum[:])[:16], "last.json")
}

// WriteRunRecord replaces the record of rec.Dir by a rename, so a reader
// never sees half of one.
func WriteRunRecord(rec protocol.RunRecord) error { //nolint:gocritic // hugeParam: once per run
	root, err := cacheRoot()
	if err != nil {
		return err
	}
	return writeRecordIn(root, rec)
}

func writeRecordIn(root string, rec protocol.RunRecord) error { //nolint:gocritic // hugeParam: once per run
	path := recordPathIn(root, rec.Dir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "last.json.tmp-*")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(append(data, '\n'))
	cerr := tmp.Close()
	if werr == nil && cerr == nil {
		werr = os.Rename(tmp.Name(), path)
	}
	if werr != nil || cerr != nil {
		_ = os.Remove(tmp.Name())
		return errors.Join(werr, cerr)
	}
	return nil
}

// ReadRunRecord reads the record of the last run made in dir; an error
// satisfying os.ErrNotExist when there is none.
func ReadRunRecord(dir string) (protocol.RunRecord, error) {
	root, err := cacheRoot()
	if err != nil {
		return protocol.RunRecord{}, err
	}
	return readRecordIn(root, dir)
}

func readRecordIn(root, dir string) (protocol.RunRecord, error) {
	var rec protocol.RunRecord
	data, err := os.ReadFile(recordPathIn(root, dir)) //nolint:gosec // G304: the record under gotest's own cache
	if err != nil {
		return rec, err
	}
	return rec, json.Unmarshal(data, &rec)
}
