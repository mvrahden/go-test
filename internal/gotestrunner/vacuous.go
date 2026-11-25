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

// observeText reads a text run's verbose output of pkg, where testing
// prints each attribute and each verdict on a line of its own; without -v
// it prints neither, and nothing is read.
func (x *assertionIndex) observeText(pkg string, out []byte) {
	if !bytes.Contains(out, attrLineMark) {
		return
	}
	for line := range bytes.Lines(out) {
		line = bytes.TrimRight(bytes.TrimLeft(line, " "), "\r\n")
		if name, ok := bytes.CutPrefix(line, runLineMark); ok {
			x.started(assertionKey{pkg, string(bytes.TrimSpace(name)), 1})
			continue
		}
		if rest, ok := bytes.CutPrefix(line, attrLineMark); ok {
			if name, kv, ok := bytes.Cut(rest, []byte(" ")); ok {
				k, v, _ := bytes.Cut(kv, []byte(" "))
				x.attr(assertionKey{pkg, string(name), 1}, string(k), string(v))
			}
			continue
		}
		for action, mark := range textVerdicts {
			if rest, ok := bytes.CutPrefix(line, mark); ok {
				if name, _, ok := bytes.Cut(rest, []byte(" (")); ok {
					x.verdict(assertionKey{pkg, string(name), 1}, action)
				}
			}
		}
	}
}

var textVerdicts = map[string][]byte{"pass": []byte("--- PASS: "), "fail": []byte("--- FAIL: "), "skip": []byte("--- SKIP: ")}

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
// does; outside CI a note says so and the code stands. It books the count
// into the run's result when the stream holds any count. A text run is
// judged only under -v, which prints the counts.
func (c *OutputCollector) judgeVacuous(ci bool, code int) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	vacuous := c.assertions.vacuous
	if c.assertions.counted {
		c.starts.vacuity(len(vacuous))
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

var (
	attrLineMark = []byte("=== ATTR  ")
	runLineMark  = []byte("=== RUN   ")
)
