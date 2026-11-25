package gotestrunner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/mvrahden/go-test/internal/protocol"
)

// assertionIndex keeps, from the JSON stream written through it, the leaves
// whose runtime reported how many assertions they made, and those of them
// that passed with none and declared no other means. A leaf no attribute
// was reported for is unknown, never vacuous.
type assertionIndex struct {
	partial []byte
	pending map[assertionKey]*leafCount
	// counted says the stream holds a count: a runtime that reports them ran.
	counted bool
	vacuous []CensusCase
	// parents holds the tests a subtest started under: no leaf, whatever
	// they reported, as one that ran subtests through t.T().Run does.
	parents map[assertionKey]bool
}

type assertionKey struct {
	pkg, test string
	instance  int
}

// leafCount is what a leaf's attributes said before its verdict came.
type leafCount struct {
	n         int
	known, by bool
}

var (
	runMarker    = []byte(`"Action":"run"`)
	attrMarker   = []byte(`"Action":"attr"`)
	resultMarker = [][]byte{[]byte(`"Action":"pass"`), []byte(`"Action":"fail"`), []byte(`"Action":"skip"`)}
)

func (x *assertionIndex) Write(p []byte) (int, error) {
	x.partial = append(x.partial, p...)
	for {
		i := bytes.IndexByte(x.partial, '\n')
		if i < 0 {
			break
		}
		x.observe(x.partial[:i])
		x.partial = x.partial[i+1:]
	}
	return len(p), nil
}

func (x *assertionIndex) observe(line []byte) {
	attr, run := bytes.Contains(line, attrMarker), bytes.Contains(line, runMarker)
	if !attr && !run && (len(x.pending) == 0 || !containsAny(line, resultMarker)) {
		return
	}
	var ev struct {
		Action, Package, Test, Key, Value string
		Gotest                            *protocol.Stamp
	}
	if json.Unmarshal(line, &ev) != nil || ev.Test == "" {
		return
	}
	key := assertionKey{ev.Package, ev.Test, 1}
	if ev.Gotest != nil && ev.Gotest.Instance > 1 {
		key.instance = ev.Gotest.Instance
	}
	switch ev.Action {
	case "run":
		x.started(key)
	case "attr":
		x.attr(key, ev.Key, ev.Value)
	case "pass", "fail", "skip":
		x.verdict(key, ev.Action)
	}
}

// started marks every test above key's as a parent.
func (x *assertionIndex) started(key assertionKey) {
	for i := range len(key.test) {
		if key.test[i] == '/' {
			if x.parents == nil {
				x.parents = map[assertionKey]bool{}
			}
			x.parents[assertionKey{key.pkg, key.test[:i], key.instance}] = true
		}
	}
}

// attr reads one attribute of key's test.
func (x *assertionIndex) attr(key assertionKey, name, value string) {
	c := x.pending[key]
	if c == nil {
		c = &leafCount{}
		if x.pending == nil {
			x.pending = map[assertionKey]*leafCount{}
		}
		x.pending[key] = c
	}
	switch name {
	case protocol.AttrAssertions:
		if n, err := strconv.Atoi(value); err == nil {
			c.n, c.known = n, true
			x.counted = true
		}
	case protocol.AttrAssertsBy:
		c.by = true
	}
}

// verdict judges key's test by its verdict and the attributes before it.
func (x *assertionIndex) verdict(key assertionKey, action string) {
	c := x.pending[key]
	if c == nil {
		return
	}
	delete(x.pending, key)
	if action == "pass" && c.known && c.n == 0 && !c.by && !x.parents[key] {
		x.vacuous = append(x.vacuous, CensusCase{Pkg: key.pkg, Path: key.test})
	}
}

// observeCounts reads the protocol.LeafCount lines a text run's process of
// pkg wrote, one per leaf that ended; a line cut short by the process's end
// is no count. A leaf that panicked reported before testing failed it, so a
// failure its stdout names and no line marks is the leaf's last run.
func (x *assertionIndex) observeCounts(pkg string, data, stdout []byte) {
	var leaves []protocol.LeafCount
	unmarked := textFailedNames(stdout)
	for line := range bytes.Lines(data) {
		var lc protocol.LeafCount
		if !bytes.HasSuffix(line, []byte("\n")) || json.Unmarshal(line, &lc) != nil || lc.Test == "" {
			continue
		}
		x.counted = true
		if lc.Failed {
			unmarked[lc.Test]--
		}
		leaves = append(leaves, lc)
	}
	for i := len(leaves) - 1; i >= 0; i-- {
		if lc := &leaves[i]; !lc.Failed && unmarked[lc.Test] > 0 {
			lc.Failed = true
			unmarked[lc.Test]--
		}
	}
	for _, lc := range leaves {
		if lc.Assertions == 0 && !lc.AssertsBy && !lc.Failed {
			x.vacuous = append(x.vacuous, CensusCase{Pkg: pkg, Path: lc.Test})
		}
	}
}

// textFailedNames counts the --- FAIL lines of a text run's stdout by test.
func textFailedNames(stdout []byte) map[string]int {
	names := map[string]int{}
	for line := range bytes.Lines(stdout) {
		rest, ok := bytes.CutPrefix(bytes.TrimLeft(line, " \t"), []byte("--- FAIL: "))
		if at := bytes.LastIndex(rest, []byte(" (")); ok && at > 0 {
			names[string(rest[:at])]++
		}
	}
	return names
}

func containsAny(line []byte, markers [][]byte) bool {
	for _, m := range markers {
		if bytes.Contains(line, m) {
			return true
		}
	}
	return false
}

// judgeVacuous names the tests that passed without an assertion: under CI
// they fail a run that is otherwise green, with exit 1, as the focus guard
// does, and are what --failed reruns; outside CI a note says so and the code
// stands. It books the count into the run's result when any test reported
// one.
func (c *OutputCollector) judgeVacuous(ci bool, code int) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	vacuous := c.assertions.vacuous
	if c.assertions.counted {
		n := len(vacuous)
		c.vacuous = &n
		c.starts.vacuity(n)
	}
	if len(vacuous) == 0 {
		return code
	}
	if !ci {
		fmt.Fprintf(c.stderr, "note: %d test(s) passed without an assertion (%s) — assert, or declare the means with t.AssertsBy(reason); --ci fails the run\n", len(vacuous), unitNames(vacuous))
		return code
	}
	fmt.Fprintf(c.stderr, "\nFAIL: %d test(s) passed without an assertion — assert, or declare the means with t.AssertsBy(reason):\n", len(vacuous))
	for _, v := range vacuous {
		fmt.Fprintf(c.stderr, "  %s %s\n", v.Pkg, v.Path)
	}
	c.vacuousFailed = vacuous
	return max(code, 1)
}

// dropCountLines drops from a text run's verbose output the lines testing
// prints for the attributes gotest reports, one per leaf.
func dropCountLines(out []byte) []byte {
	if !bytes.Contains(out, attrLineMark) {
		return out
	}
	var kept []byte
	for line := range bytes.Lines(out) {
		if rest, ok := bytes.CutPrefix(bytes.TrimLeft(line, " "), attrLineMark); ok {
			if _, attr, _ := bytes.Cut(rest, []byte(" ")); bytes.HasPrefix(attr, []byte("gotest.")) {
				continue
			}
		}
		kept = append(kept, line...)
	}
	return kept
}

var attrLineMark = []byte("=== ATTR  ")
