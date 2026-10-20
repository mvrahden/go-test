package speccatalog

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// JudgeRun is one judge invocation for a drill mutant: the packages it tests
// and the "<Suite>/<Method>" tests it selects, none selecting all of them.
type JudgeRun struct {
	Packages []string
	Tests    []string
}

// RunFilter is the -run pattern selecting r's tests, empty when r selects
// every test. Suites and methods are matched level by level, as go test
// splits -run on "/", so several suites select every listed method in each.
func (r JudgeRun) RunFilter() string {
	if len(r.Tests) == 0 {
		return ""
	}
	var suites, methods []string
	// A nested selection is emitted as its own run, avoiding a cross-product
	// of unrelated method and behavior paths.
	if len(r.Tests) == 1 {
		parts := strings.Split("Test"+r.Tests[0], "/")
		for i := range parts {
			parts[i] = "^(" + regexp.QuoteMeta(parts[i]) + ")$"
		}
		return strings.Join(parts, "/")
	}
	for _, t := range r.Tests {
		suite, method, _ := strings.Cut(t, "/")
		suites = appendNew(suites, "Test"+suite)
		methods = appendNew(methods, method)
	}
	return "^(" + strings.Join(suites, "|") + ")$/^(" + strings.Join(methods, "|") + ")$"
}

func appendNew(list []string, s string) []string {
	for _, x := range list {
		if x == s {
			return list
		}
	}
	return append(list, s)
}

// conformanceExpect reads the capability off an Expect line anchored to a
// conformance package's FAIL line.
var conformanceExpect = regexp.MustCompile(`^\^FAIL \.\*conformance/([a-z0-9]+)[ /]`)

// testName is a Judge: line's "<Suite>/<Method>".
var testName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*TestSuite/Test[A-Za-z0-9_]*(/[^/[:space:]]+)*$`)

// JudgeRuns reads a drill patch's header and returns the runs that judge it,
// and every reason it cannot be judged.
//
// An Expect line anchored to a conformance package's FAIL line is judged by
// that package, selecting the suites and methods holding the patch's Clause
// lines of its capability, or all of them when it names none. Every other
// Expect line is judged by the patch's Judge: lines, each one run of a
// package directory and optionally the "<Suite>/<Method>" tests it selects.
// An Expect line neither route reaches is refused, not run against a
// default: only a run of the named check may catch the mutant.
func JudgeRuns(specs []Spec, catalog []byte, patch []byte) ([]JudgeRun, []string) {
	h := readHeader(patch)
	var problems []string
	known := map[string]bool{}
	for _, c := range Clauses(specs) {
		known[c] = true
	}
	for _, c := range h.clauses {
		if !known[c] {
			problems = append(problems, "Clause line names no clause of specs/: "+c)
		}
	}
	tests, err := clauseTests(catalog)
	if err != nil {
		return nil, append(problems, err.Error())
	}

	var caps []string
	for _, e := range h.expects {
		switch m := conformanceExpect.FindStringSubmatch(e); {
		case m != nil:
			caps = appendNew(caps, m[1])
		case strings.Contains(e, "conformance/"):
			problems = append(problems, "Expect line names a conformance package but is not anchored to its FAIL line (^FAIL .*conformance/<capability> <Suite> ...): "+e)
		case len(h.judges) == 0:
			problems = append(problems, "Expect line names no conformance failure and no Judge: line names the check that prints it: "+e)
		}
	}
	if len(caps) > 0 && LinksIntoTestBinaries(patch) {
		problems = append(problems, "an Expect line names a conformance failure, but the mutant lives in what test binaries link, where the conformance suites assert with the mutated code")
	}
	sort.Strings(caps)

	var runs []JudgeRun
	explicit := map[string]bool{}
	for _, line := range h.judges {
		fields := strings.Fields(line)
		if len(fields) == 0 || !strings.HasPrefix(fields[0], "./") {
			problems = append(problems, "Judge: line names no package directory: "+line)
			continue
		}
		for _, t := range fields[1:] {
			if !testName.MatchString(t) {
				problems = append(problems, fmt.Sprintf("Judge: line names %q, not a <Suite>/<Method>", t))
			}
		}
		explicit[fields[0]] = true
		methods := []string{}
		for _, test := range fields[1:] {
			if strings.Count(test, "/") > 1 {
				// Literal subtest names, as rendered in catalog.json, not regex.
				if strings.HasPrefix(fields[0], "./"+ConformanceDir+"/") && !catalogTestPath(catalog, fields[0], test) {
					problems = append(problems, "catalog declares no selected behavior: "+test)
				}
				runs = append(runs, JudgeRun{Packages: fields[:1], Tests: []string{test}})
			} else {
				methods = append(methods, test)
			}
		}
		if len(methods) > 0 || len(fields) == 1 {
			runs = append(runs, JudgeRun{Packages: fields[:1], Tests: methods})
		}
	}
	for _, c := range caps {
		if explicit["./"+ConformanceDir+"/"+c] {
			continue
		}
		var selected []string
		for _, clause := range h.clauses {
			for _, t := range tests[c][clause] {
				selected = appendNew(selected, t)
			}
		}
		runs = append(runs, JudgeRun{Packages: []string{"./" + ConformanceDir + "/" + c}, Tests: selected})
	}
	for _, why := range h.alone {
		if strings.TrimSpace(why) == "" {
			problems = append(problems, "Alone: line gives no reason")
		}
	}
	if len(h.expects) == 0 {
		problems = append(problems, "no Expect: line, name the check that must catch it")
	}
	return runs, problems
}

type header struct {
	expects, clauses, judges, alone []string
}

// AskedAlone reports whether the patch carries an "Alone: <why>" line: its
// checks read the clock in a suite that does not declare Exclusive, so the
// mutant runs alone, after the others, as one an Exclusive suite judges does.
func AskedAlone(patch []byte) bool {
	for _, why := range readHeader(patch).alone {
		if strings.TrimSpace(why) != "" {
			return true
		}
	}
	return false
}

// readHeader collects the Expect:, Clause:, Judge: and Alone: lines above the diff.
func readHeader(patch []byte) header {
	var h header
	for _, line := range strings.Split(string(patch), "\n") {
		if strings.HasPrefix(line, "--- ") {
			break
		}
		if v, ok := strings.CutPrefix(line, "Expect: "); ok {
			h.expects = append(h.expects, v)
		} else if v, ok := strings.CutPrefix(line, "Clause: "); ok {
			h.clauses = append(h.clauses, v)
		} else if v, ok := strings.CutPrefix(line, "Judge: "); ok {
			h.judges = append(h.judges, v)
		} else if v, ok := strings.CutPrefix(line, "Alone:"); ok {
			h.alone = append(h.alone, v)
		}
	}
	return h
}

// clauseTests maps each capability's clauses, spelled as a Clause: line
// spells them, to the "<Suite>/<Method>" tests that state them.
func clauseTests(js []byte) (map[string]map[string][]string, error) {
	var doc struct {
		Packages []struct {
			Path  string `json:"path"`
			Nodes []node `json:"nodes"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(js, &doc); err != nil {
		return nil, fmt.Errorf("catalog json: %w", err)
	}
	out := map[string]map[string][]string{}
	for _, p := range doc.Packages {
		_, capability, _ := strings.Cut(p.Path, "/"+ConformanceDir+"/")
		if out[capability] == nil {
			out[capability] = map[string][]string{}
		}
		for _, suite := range p.Nodes {
			if suite.Excluded {
				continue
			}
			for i := range suite.Children {
				method := &suite.Children[i]
				test := strings.TrimPrefix(suite.Name, "Test") + "/" + method.Name
				walk(method, nil, func(when, it *node) {
					c := clauseWhen(whenLabel(when.Display, when.Name)) + Arrow + strings.TrimSpace(it.Display)
					out[capability][c] = appendNew(out[capability][c], test)
				})
			}
		}
	}
	return out, nil
}

// Suites indexes the suites declared in a package's test files.
type Suites struct {
	// Methods holds every "<Suite>/<Method>" declared.
	Methods map[string]bool
	// Exclusive holds the suites whose SuiteConfig sets Exclusive.
	Exclusive map[string]bool
}

// ReadSuites indexes the test files of the package directories pattern
// names under root: one directory, or every one below it for a pattern
// ending in "/...", testdata excluded as go test excludes it.
func ReadSuites(root, pattern string) (Suites, error) {
	s := Suites{Methods: map[string]bool{}, Exclusive: map[string]bool{}}
	dir, recursive := strings.CutSuffix(pattern, "/...")
	dir = filepath.Join(root, filepath.FromSlash(dir))
	if _, err := os.Stat(dir); err != nil {
		return s, err
	}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != dir && (!recursive || d.Name() == "testdata" || strings.HasPrefix(d.Name(), ".") || strings.HasPrefix(d.Name(), "_")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		return s.read(path)
	})
	return s, err
}

func (s Suites) read(path string) error {
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
	if err != nil {
		return err
	}
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || len(fn.Recv.List) != 1 {
			continue
		}
		star, ok := fn.Recv.List[0].Type.(*ast.StarExpr)
		if !ok {
			continue
		}
		recv, ok := star.X.(*ast.Ident)
		if !ok {
			continue
		}
		s.Methods[recv.Name+"/"+fn.Name.Name] = true
		if fn.Name.Name == "SuiteConfig" && fn.Body != nil && setsExclusive(fn.Body) {
			s.Exclusive[recv.Name] = true
		}
	}
	return nil
}

// setsExclusive reports whether body sets an Exclusive field to anything
// but false, in a composite literal or by assignment.
func setsExclusive(body *ast.BlockStmt) bool {
	found := false
	notFalse := func(e ast.Expr) bool {
		id, ok := e.(*ast.Ident)
		return !ok || id.Name != "false"
	}
	ast.Inspect(body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.KeyValueExpr:
			if k, ok := n.Key.(*ast.Ident); ok && k.Name == "Exclusive" && notFalse(n.Value) {
				found = true
			}
		case *ast.AssignStmt:
			for i, lhs := range n.Lhs {
				if sel, ok := lhs.(*ast.SelectorExpr); ok && sel.Sel.Name == "Exclusive" && i < len(n.Rhs) && notFalse(n.Rhs[i]) {
					found = true
				}
			}
		}
		return !found
	})
	return found
}

// Alone reports whether r runs a suite whose SuiteConfig sets Exclusive:
// its verdicts read the wall clock, so it must not share the machine with
// another mutant's run. Each problem names a selected test r's packages do
// not declare.
func (r JudgeRun) Alone(root string) (bool, []string, error) {
	all := Suites{Methods: map[string]bool{}, Exclusive: map[string]bool{}}
	for _, p := range r.Packages {
		s, err := ReadSuites(root, p)
		if err != nil {
			return false, nil, err
		}
		for k := range s.Methods {
			all.Methods[k] = true
		}
		for k := range s.Exclusive {
			all.Exclusive[k] = true
		}
	}
	if len(r.Tests) == 0 {
		return len(all.Exclusive) > 0, nil, nil
	}
	alone := false
	var problems []string
	for _, t := range r.Tests {
		suite, _, _ := strings.Cut(t, "/")
		parts := strings.Split(t, "/")
		method := strings.Join(parts[:min(2, len(parts))], "/")
		if !all.Methods[method] {
			problems = append(problems, fmt.Sprintf("%s declares no %s", strings.Join(r.Packages, " "), t))
		}
		alone = alone || all.Exclusive[suite]
	}
	return alone, problems, nil
}

// catalogTestPath validates an explicitly selected nested behavior against the
// pristine catalog, so a typo cannot silently select no row.
func catalogTestPath(data []byte, pkg, test string) bool {
	var doc struct {
		Packages []struct {
			Path  string `json:"path"`
			Nodes []node `json:"nodes"`
		} `json:"packages"`
	}
	if json.Unmarshal(data, &doc) != nil {
		return false
	}
	var contains func([]node, []string) bool
	contains = func(nodes []node, parts []string) bool {
		for _, n := range nodes {
			if !n.Excluded && n.Name == parts[0] {
				if len(parts) == 1 || contains(n.Children, parts[1:]) {
					return true
				}
			}
		}
		return false
	}
	for _, p := range doc.Packages {
		if strings.HasSuffix(p.Path, strings.TrimPrefix(pkg, ".")) && contains(p.Nodes, strings.Split("Test"+test, "/")) {
			return true
		}
	}
	return false
}
