package lint_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// LinterRewritesTestSuite: an assertion lint offers to rewrite keeps its
// verdict once rewritten; where the operand's type would change it, lint
// offers no fix.
type LinterRewritesTestSuite struct {
	dir string
	// per row, by package name
	findings map[string][]string
	before   map[string]string
	after    map[string]string
	fixErr   map[string]string
	fixed    map[string]string
}

func (s *LinterRewritesTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

// rewriteRow is one operand type under one assertion, staged as a package of
// its own with a single test.
type rewriteRow struct {
	Desc    string
	group   string
	imports []string
	decls   string
	body    string
	// rules are the lint categories the assertion is reported under.
	rules []string
	// want is the assertion line after lint -fix; empty when no fix applies.
	want string
	// verdict is the test's verdict before the fix, and after it.
	verdict string
}

const (
	groupConcrete  = "concrete"
	groupInterface = "interface"
	groupCore      = "core"
	groupBool      = "bool"
	groupErrorMsg  = "errormsg"
	groupOrdering  = "ordering"
)

const simplify = "assertion-simplify"

const myErr = "type myErr struct{}\n\nfunc (*myErr) Error() string { return \"boom\" }\n"

var rewriteRows = []rewriteRow{
	// A typed nil pointer is not a nil error once converted to one.
	{Desc: "True of a nil *myErr == nil", group: groupConcrete, decls: myErr,
		body: "var p *myErr\n\tgotest.True(t, p == nil)", rules: []string{simplify}, want: "gotest.Zero(t, p)", verdict: "pass"},
	{Desc: "Equal of nil and a nil *myErr", group: groupConcrete, decls: myErr,
		body: "var p *myErr\n\tgotest.Equal(t, nil, p)", rules: []string{simplify}, want: "gotest.Zero(t, p)", verdict: "pass"},
	{Desc: "Nil of a nil *myErr", group: groupConcrete, decls: myErr,
		body: "var p *myErr\n\tgotest.Nil(t, p)", rules: []string{simplify}, want: "gotest.Zero(t, p)", verdict: "pass"},
	{Desc: "Zero of a nil *myErr", group: groupConcrete, decls: myErr,
		body: "var p *myErr\n\tgotest.Zero(t, p)", verdict: "pass"},
	{Desc: "Empty of a nil *myErr", group: groupConcrete, decls: myErr,
		body: "var p *myErr\n\tgotest.Empty(t, p)", verdict: "pass"},
	{Desc: "Nil of a type parameter constrained by error", group: groupConcrete, decls: myErr + "\nfunc check[E error](t *testing.T, e E) {\n\tgotest.Nil(t, e)\n}\n",
		body: "check[*myErr](t, nil)", verdict: "pass"},
	{Desc: "True of a nil error == nil", group: groupConcrete,
		body: "var err error\n\tgotest.True(t, err == nil)", rules: []string{simplify}, want: "gotest.NoError(t, err)", verdict: "pass"},
	{Desc: "Zero of an error holding a nil *myErr", group: groupConcrete, decls: myErr,
		body: "var err error = (*myErr)(nil)\n\tgotest.Zero(t, err)", rules: []string{simplify}, want: "gotest.NoError(t, err)", verdict: "fail"},

	// Nil looks through an interface; Zero and NoError do not.
	{Desc: "Nil of an error holding a nil *myErr", group: groupInterface, decls: myErr,
		body: "var err error = (*myErr)(nil)\n\tgotest.Nil(t, err)", rules: []string{simplify}, verdict: "pass"},
	{Desc: "NotNil of an error holding a nil *myErr", group: groupInterface, decls: myErr,
		body: "var err error = (*myErr)(nil)\n\tgotest.NotNil(t, err)", rules: []string{simplify}, verdict: "fail"},
	{Desc: "Nil of an any holding a nil *int", group: groupInterface,
		body: "var v any = (*int)(nil)\n\tgotest.Nil(t, v)", rules: []string{simplify}, verdict: "pass"},
	{Desc: "Empty of an error holding a nil *myErr", group: groupInterface, decls: myErr,
		body: "var err error = (*myErr)(nil)\n\tgotest.Empty(t, err)", rules: []string{simplify}, verdict: "pass"},
	{Desc: "True of an any holding a nil *int == nil", group: groupInterface,
		body: "var v any = (*int)(nil)\n\tgotest.True(t, v == nil)", rules: []string{simplify}, want: "gotest.Zero(t, v)", verdict: "fail"},

	// A type parameter reads as its core type; unsafe.Pointer is nilable.
	{Desc: "Nil of a type parameter over *int", group: groupCore, decls: "func check[P ~*int](t *testing.T, p P) {\n\tgotest.Nil(t, p)\n}\n",
		body: "check[*int](t, nil)", rules: []string{simplify}, want: "gotest.Zero(t, p)", verdict: "pass"},
	{Desc: "True of a type parameter over []int == nil", group: groupCore, decls: "func check[P ~[]int](t *testing.T, p P) {\n\tgotest.True(t, p == nil)\n}\n",
		body: "check[[]int](t, nil)", rules: []string{simplify}, want: "gotest.Nil(t, p)", verdict: "pass"},
	{Desc: "Nil of an unsafe.Pointer", group: groupCore, imports: []string{"unsafe"},
		body: "var up unsafe.Pointer\n\tgotest.Nil(t, up)", rules: []string{simplify}, want: "gotest.Zero(t, up)", verdict: "pass"},

	// True takes a bool, nothing else.
	{Desc: "Equal of true and a bool", group: groupBool,
		body: "b := true\n\tgotest.Equal(t, true, b)", rules: []string{simplify}, want: "gotest.True(t, b)", verdict: "pass"},
	{Desc: "Equal of true and a named bool", group: groupBool, decls: "type flag bool\n",
		body: "var f flag = true\n\tgotest.Equal(t, true, f)", verdict: "pass"},
	{Desc: "Equal of true and an any holding a bool", group: groupBool,
		body: "var v any = true\n\tgotest.Equal(t, true, v)", verdict: "pass"},

	// ErrorContains takes an error.
	{Desc: "Contains of an error's message", group: groupErrorMsg, imports: []string{"errors"},
		body: "err := errors.New(\"boom\")\n\tgotest.Contains(t, err.Error(), \"oo\")", rules: []string{simplify}, want: "gotest.ErrorContains(t, err, \"oo\")", verdict: "pass"},
	{Desc: "Contains of a message from a value that does not implement error", group: groupErrorMsg, decls: myErr,
		body: "var v myErr\n\tgotest.Contains(t, v.Error(), \"oo\")", verdict: "pass"},

	// cmp.Compare orders NaN below every number; the operators do not.
	{Desc: "False of an int >", group: groupOrdering,
		body: "i, j := 1, 2\n\tgotest.False(t, i > j)", rules: []string{simplify}, want: "gotest.LessOrEqual(t, i, j)", verdict: "pass"},
	{Desc: "False of a float > NaN", group: groupOrdering, imports: []string{"math"},
		body: "x, y := 1.0, math.NaN()\n\tgotest.False(t, x > y)", verdict: "pass"},
	{Desc: "True of NaN < a float", group: groupOrdering, imports: []string{"math"},
		body: "x, y := math.NaN(), 1.0\n\tgotest.True(t, x < y)", verdict: "fail"},
}

// rowPkg names the package row i is staged in.
func rowPkg(i int) string { return fmt.Sprintf("row%02d", i) }

func rowSource(i int, row *rewriteRow) string {
	var b strings.Builder
	fmt.Fprintf(&b, "package %s\n\nimport (\n\t\"testing\"\n", rowPkg(i))
	for _, imp := range row.imports {
		fmt.Fprintf(&b, "\t%q\n", imp)
	}
	b.WriteString("\n\t\"github.com/mvrahden/go-test/pkg/gotest\"\n)\n\n")
	if row.decls != "" {
		b.WriteString(row.decls + "\n")
	}
	fmt.Fprintf(&b, "func TestRow(t *testing.T) {\n\t%s\n}\n", row.body)
	return b.String()
}

// BeforeAll stages every row, lints them, records each row's verdict, applies
// lint's fixes, then vets and runs every row again.
func (s *LinterRewritesTestSuite) BeforeAll(t *gotest.T) {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	s.dir, err = os.MkdirTemp("", "gotest-rewrites-")
	gotest.NoError(t, err)
	gotest.NoError(t, testkit.WriteModule(root, s.dir, "rewrites"))
	for i := range rewriteRows {
		pkg := filepath.Join(s.dir, rowPkg(i))
		gotest.NoError(t, os.MkdirAll(pkg, 0o750))
		gotest.NoError(t, os.WriteFile(filepath.Join(pkg, "row_test.go"), []byte(rowSource(i, &rewriteRows[i])), 0o600))
	}

	s.before = rowVerdicts(t, s.dir)
	s.findings = rowFindings(t, s.dir)

	// Each row is fixed alone, so its verdict reads only its own fix.
	s.fixed = map[string]string{}
	s.fixErr = map[string]string{}
	fixes := make([]string, len(rewriteRows))
	sources := make([]string, len(rewriteRows))
	slots := make(chan struct{}, 8)
	waits := make([]func(), len(rewriteRows))
	for i := range rewriteRows {
		waits[i] = gotest.Go(t, func() {
			slots <- struct{}{}
			defer func() { <-slots }()
			fixes[i] = fixRow(t, s.dir, rowPkg(i))
			src, err := os.ReadFile(filepath.Join(s.dir, rowPkg(i), "row_test.go"))
			gotest.NoError(t, err)
			sources[i] = string(src)
		})
	}
	for i, wait := range waits {
		wait()
		s.fixed[rowPkg(i)] = sources[i]
		s.fixErr[rowPkg(i)] = fixes[i]
	}
	s.after = rowVerdicts(t, s.dir)
}

func (s *LinterRewritesTestSuite) AfterAll(t *gotest.T) {
	if s.dir != "" {
		gotest.NoError(t, os.RemoveAll(s.dir))
	}
}

func (s *LinterRewritesTestSuite) TestNilChecks(t *gotest.T) {
	t.When("an assertion checks an operand against nil", func(w *gotest.T) {
		w.It("a nil comparison of a concrete pointer is not rewritten into an error assertion", func(it *gotest.T) {
			s.checkRows(it, groupConcrete)
		})
		w.It("a Nil check of an interface is reported without a fix", func(it *gotest.T) {
			s.checkRows(it, groupInterface)
		})
		w.It("a type parameter is read as its core type and unsafe.Pointer as nilable", func(it *gotest.T) {
			s.checkRows(it, groupCore)
		})
	})
}

func (s *LinterRewritesTestSuite) TestBoolLiterals(t *gotest.T) {
	t.When("an assertion compares an operand with a bool literal", func(w *gotest.T) {
		w.It("the comparison is rewritten into True or False only for an operand of type bool", func(it *gotest.T) {
			s.checkRows(it, groupBool)
		})
	})
}

func (s *LinterRewritesTestSuite) TestErrorMessages(t *gotest.T) {
	t.When("an assertion searches the result of an Error call", func(w *gotest.T) {
		w.It("the search is rewritten into ErrorContains only on a receiver that implements error", func(it *gotest.T) {
			s.checkRows(it, groupErrorMsg)
		})
	})
}

func (s *LinterRewritesTestSuite) TestOrderings(t *gotest.T) {
	t.When("an assertion states an ordering comparison", func(w *gotest.T) {
		w.It("a float comparison is not rewritten into an ordering assertion", func(it *gotest.T) {
			s.checkRows(it, groupOrdering)
		})
	})
}

// checkRows holds each row of group to its findings, its fix, and a verdict
// the fix leaves as it was.
func (s *LinterRewritesTestSuite) checkRows(t *gotest.T, group string) {
	type indexed struct {
		Desc string
		i    int
		row  *rewriteRow
	}
	var rows []indexed
	for i := range rewriteRows {
		if row := &rewriteRows[i]; row.group == group {
			rows = append(rows, indexed{Desc: row.Desc, i: i, row: row})
		}
	}
	gotest.NotEmpty(t, rows)
	for t, r := range gotest.Each(t, rows) {
		pkg := rowPkg(r.i)
		src := rowSource(r.i, r.row)
		gotest.Equal(t, r.row.verdict, s.before[pkg], "source:\n%s", src)
		fixed := s.fixed[pkg]
		gotest.Empty(t, s.fixErr[pkg], "fixed source:\n%s", fixed)
		gotest.Equal(t, s.before[pkg], s.after[pkg], "fixed source:\n%s", fixed)
		if r.row.want == "" {
			gotest.Equal(t, src, fixed)
		} else {
			gotest.Contains(t, fixed, "\t"+r.row.want+"\n")
		}
		gotest.Equal(t, r.row.rules, s.findings[pkg], "source:\n%s", src)
	}
}

// rowFindings lints dir and returns the categories of the assertion rules
// reported in each row's package, sorted.
func rowFindings(t *gotest.T, dir string) map[string][]string {
	run, err := testkit.RunCLI(t.Context(), dir, "lint", "-json", "./...")
	gotest.NoError(t, err)
	var byPkg map[string]map[string][]lintDiagnostic
	gotest.NoError(t, json.Unmarshal([]byte(run.Stdout), &byPkg), "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
	got := map[string][]string{}
	for _, analyzers := range byPkg {
		for _, d := range analyzers["gotestlint"] {
			if !strings.HasPrefix(d.Category, "assertion-") {
				continue
			}
			file, _, _ := strings.Cut(d.Posn, ":")
			pkg := filepath.Base(filepath.Dir(file))
			if !slices.Contains(got[pkg], d.Category) {
				got[pkg] = append(got[pkg], d.Category)
			}
		}
	}
	for _, cats := range got {
		slices.Sort(cats)
	}
	return got
}

// rowVerdicts runs every row with go test -json and returns TestRow's verdict
// per package; a package that does not build has none.
func rowVerdicts(t *gotest.T, dir string) map[string]string {
	cmd := exec.CommandContext(t.Context(), "go", "test", "-json", "-count=1", "./...")
	cmd.Dir = dir
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	_ = cmd.Run() // failing rows exit non-zero; their verdicts are the result
	verdicts := map[string]string{}
	for _, ev := range testkit.ParseEvents(stdout.Bytes()) {
		if ev.Test != "TestRow" {
			continue
		}
		switch ev.Action {
		case "pass", "fail", "skip":
			verdicts[filepath.Base(ev.Package)] = ev.Action
		}
	}
	gotest.NotEmpty(t, verdicts, "go test output:\n%s", stdout.String())
	return verdicts
}

// fixRow applies lint's fixes to pkg in dir and vets it, returning what
// went wrong; empty when the fixed package is clean.
func fixRow(t *gotest.T, dir, pkg string) string {
	fix, err := testkit.RunCLI(t.Context(), dir, "lint", "-fix", "./"+pkg)
	gotest.NoError(t, err)
	// 0: nothing left; 3: findings left that offer no fix.
	if fix.Exit != 0 && fix.Exit != 3 {
		return fmt.Sprintf("lint -fix exit %d: %s", fix.Exit, fix.Stderr)
	}
	cmd := exec.CommandContext(t.Context(), "go", "vet", "./"+pkg) //nolint:gosec // G204: a package this suite staged
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return "go vet: " + string(out)
	}
	return ""
}
