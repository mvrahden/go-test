package mutate_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/tools/go/packages"

	"github.com/mvrahden/go-test/internal/mutate"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SchemataTestSuite: one build holds every mutant of the changed lines, each
// type-checked; the id the binary reads switches one on, none the source.
// The fixtures live in testdata/ops, built in place through an overlay.
type SchemataTestSuite struct{}

func (s *SchemataTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 60 * time.Second}
}

// mut is a mutant as the suite states it by hand.
type mut struct {
	Line              int
	Func, Op          string
	Original, Mutated string
}

// fixture is a package of testdata/ops loaded with every line changed.
type fixture struct {
	dir, file string
	plan      *mutate.Plan
}

func load(t *gotest.T, pkg string, opt mutate.Options) fixture {
	dir, err := filepath.Abs(filepath.Join("testdata", "ops"))
	gotest.NoError(t, err)
	pkgs, err := packages.Load(&packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo,
		Dir:  dir,
		Env:  append(os.Environ(), "GOWORK=off"),
	}, "./"+pkg)
	gotest.NoError(t, err)
	gotest.Len(t, pkgs, 1)
	gotest.Len(t, pkgs[0].CompiledGoFiles, 1)
	file := pkgs[0].CompiledGoFiles[0]
	src, err := os.ReadFile(file)
	gotest.NoError(t, err)
	every := map[int]bool{}
	for l := 1; l <= strings.Count(string(src), "\n"); l++ {
		every[l] = true
	}
	plan, err := mutate.Generate(pkgs, mutate.Changed{file: every}, opt)
	gotest.NoError(t, err)
	return fixture{dir: dir, file: file, plan: plan}
}

func (f *fixture) mutants() []mut {
	var out []mut
	for _, m := range f.plan.Mutants {
		out = append(out, mut{m.Line, m.Func, m.Op, m.Original, m.Mutated})
	}
	return out
}

// build compiles the fixture's package with the plan's overlay into dir
// and returns the compiler's output.
func (f *fixture) build(t *gotest.T, pkg, out string) ([]byte, error) {
	replace := map[string]string{}
	tmp := t.TempDir()
	for name, src := range f.plan.Files {
		p := filepath.Join(tmp, filepath.Base(name))
		gotest.NoError(t, os.WriteFile(p, src, 0o600))
		replace[name] = p
	}
	data, err := json.Marshal(map[string]any{"Replace": replace})
	gotest.NoError(t, err)
	overlay := filepath.Join(tmp, "overlay.json")
	gotest.NoError(t, os.WriteFile(overlay, data, 0o600))
	build := exec.CommandContext(t.Context(), "go", "build", "-overlay="+overlay, "-o", out, "./"+pkg) //nolint:gosec // G204: go build of the suite's own fixture
	build.Dir = f.dir
	build.Env = append(os.Environ(), "GOWORK=off")
	return build.CombinedOutput()
}

func (s *SchemataTestSuite) TestEveryMutantSwitchesOnAlone(t *gotest.T) {
	f := load(t, "run", mutate.Options{})
	bin := filepath.Join(t.TempDir(), "run")
	out, err := f.build(t, "run", bin)
	gotest.NoError(t, err, "%s", out)
	runAs := func(id int) string {
		cmd := exec.CommandContext(t.Context(), bin)
		cmd.Env = append(os.Environ(), mutate.EnvMutant+"="+strconv.Itoa(id))
		out, err := cmd.Output()
		gotest.NoError(t, err)
		return strings.TrimSpace(string(out))
	}
	const source = "22 true 5 1.5 1 true"
	t.When("the changed lines hold a method, a generic function, a closure and constants", func(w *gotest.T) {
		w.It("writes each mutant the operators allow, in source order, and none in a constant", func(it *gotest.T) {
			gotest.Equal(it, []mut{
				{12, "box.add", mutate.OpReturn, "b.n = b.n + v", "return"},
				{12, "box.add", mutate.OpDelete, "b.n = b.n + v", ""},
				{16, "maxOf", mutate.OpReturn, "if a > b { return a }", "return *new(T)"},
				{16, "maxOf", mutate.OpBoundary, "a > b", "a >= b"},
				{16, "maxOf", mutate.OpNegate, "a > b", "a <= b"},
				{23, "main", mutate.OpReturn, "var arr [limit]int", "return"},
				{25, "main", mutate.OpDelete, "b.add(1)", ""},
				{25, "main", mutate.OpLiteral, "1", "0"},
				{26, "main", mutate.OpLogical, "b.n == 1 && len(arr) == limit", "b.n == 1 || len(arr) == limit"},
				{26, "main", mutate.OpNegate, "b.n == 1", "b.n != 1"},
				{26, "main", mutate.OpLiteral, "1", "0"},
				{27, "main", mutate.OpReturn, "return x + 10", "return 0"},
				{28, "main", mutate.OpLiteral, "0", "1"},
				{28, "main", mutate.OpBoundary, "i < 2", "i <= 2"},
				{28, "main", mutate.OpNegate, "i < 2", "i >= 2"},
				{29, "main", mutate.OpDelete, "b.add(double(i))", ""},
				{32, "main", mutate.OpDelete, "fmt.Println(b.n, ok, maxOf(2, 5), maxOf(1.5, 0.5), lv, true)", ""},
				{32, "main", mutate.OpLiteral, "true", "false"},
			}, f.mutants())
		})
	})
	t.When("no mutant is switched on", func(w *gotest.T) {
		w.It("behaves as the source", func(it *gotest.T) {
			gotest.Equal(it, source, runAs(0))
		})
	})
	t.When("each mutant is switched on in turn", func(w *gotest.T) {
		w.It("changes the output, but for the two this input cannot tell apart", func(it *gotest.T) {
			var same []string
			for _, m := range f.plan.Mutants {
				if runAs(m.ID) == source {
					same = append(same, m.Op+" "+m.Mutated)
				}
			}
			gotest.Equal(it, []string{
				"boundary a >= b",                       // maxOf is never given equal values
				"logical b.n == 1 || len(arr) == limit", // its right operand is always true
			}, same)
		})
	})
}

func (s *SchemataTestSuite) TestWhatNoOperatorTakes(t *gotest.T) {
	t.When("the changed lines hold what a mutant cannot be written in", func(w *gotest.T) {
		f := load(t, "refused", mutate.Options{})
		w.It("writes none in a constant, an array length or key, a named or type-parameter literal, a header statement, a declaring assignment, a panic, a multi-line operand or before a labeled statement", func(it *gotest.T) {
			gotest.Equal(it, []mut{
				{13, "Warm", mutate.OpBoundary, "c < 20", "c <= 20"},
				{13, "Warm", mutate.OpNegate, "c < 20", "c >= 20"},
				{14, "Warm", mutate.OpDelete, "c++", ""},
				{17, "Warm", mutate.OpNegate, "c == celsius(limit)", "c != celsius(limit)"},
				{21, "Pause", mutate.OpReturn, "return time.Duration(1) * time.Millisecond", "return 0"},
				{25, "Span", mutate.OpReturn, "var zero T = 0", "return *new(T)"},
				{27, "Span", mutate.OpLiteral, "1", "0"}, // an int converted to T
				{27, "Span", mutate.OpBoundary, "i < n", "i <= n"},
				{27, "Span", mutate.OpNegate, "i < n", "i >= n"},
				{28, "Span", mutate.OpDelete, "total += i", ""},
				{34, "Long", mutate.OpReturn, "return a < b", "return false"},
				{39, "Fail", mutate.OpReturn, `panic("refused")`, "return"},
			}, f.mutants())
		})
		w.It("compiles them all in one build", func(it *gotest.T) {
			out, err := f.build(it, "refused", filepath.Join(it.TempDir(), "refused.a"))
			gotest.NoError(it, err, "%s", out)
		})
	})
	t.When("string literals are asked for", func(w *gotest.T) {
		w.It("adds a mutant that empties each string", func(it *gotest.T) {
			f := load(it, "refused", mutate.Options{Strings: true})
			var strs []mut
			for _, m := range f.mutants() {
				if m.Op == mutate.OpString {
					strs = append(strs, m)
				}
			}
			gotest.Equal(it, []mut{{39, "Fail", mutate.OpString, `"refused"`, `""`}}, strs)
		})
	})
	t.When("a mutant is dropped", func(w *gotest.T) {
		w.It("leaves its site as written and keeps the others' ids", func(it *gotest.T) {
			all := load(it, "refused", mutate.Options{})
			dropped := load(it, "refused", mutate.Options{Drop: map[int]bool{all.plan.Mutants[0].ID: true}})
			gotest.Equal(it, all.plan.Mutants[1:], dropped.plan.Mutants)
			gotest.NotContains(it, string(dropped.plan.Files[dropped.file]), "gotestMutantID == 1 &&")
		})
	})
}
