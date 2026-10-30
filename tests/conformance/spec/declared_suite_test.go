package spec_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SpecDeclaredTestSuite: --declared lays a stream's verdicts over the
// specification the source declares, and the source's positions travel
// with the nodes it declares.
type SpecDeclaredTestSuite struct {
	dir string
	// input is a stream of the methods named TestRan…, written by a run.
	input string
}

func (s *SpecDeclaredTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 45 * time.Second, SetupTimeout: 90 * time.Second}
}

const declaredPkg = "../testdata/declared/"

func (s *SpecDeclaredTestSuite) BeforeAll(t *gotest.T) {
	dir, err := os.MkdirTemp("", "gotest-declared-")
	gotest.NoError(t, err)
	s.dir = dir
	run, err := testkit.RunPackage(t.Context(), dir, declaredPkg, "-run", "TestDeclaredTestSuite/TestRan")
	gotest.NoError(t, err)
	gotest.Equal(t, 0, run.Exit, "stderr: %s", run.Stderr)
	s.input = filepath.Join(dir, "events.jsonl")
	gotest.NoError(t, os.WriteFile(s.input, []byte(run.Stdout), 0o600))
}

func (s *SpecDeclaredTestSuite) AfterAll(t *gotest.T) {
	if s.dir != "" {
		gotest.NoError(t, os.RemoveAll(s.dir))
	}
}

func (s *SpecDeclaredTestSuite) TestARunOfSomeMethods(t *gotest.T) {
	t.When("a stream of some of a package's methods is rendered with --declared", func(w *gotest.T) {
		w.It("shows every method the source declares with its When and It rows, those the stream did not report with no verdict and no duration", func(it *gotest.T) {
			doc := specJSON(it, 0, "--input="+s.input, "--declared", declaredPkg)
			gotest.Len(it, doc.Packages, 1)
			pkg := doc.Packages[0]
			gotest.Equal(it, "pass", node(it, pkg.Nodes, "TestDeclaredTestSuite", "TestRanAlone", "the_method_runs", "passes").Status)
			left := node(it, pkg.Nodes, "TestDeclaredTestSuite", "TestLeftOut")
			gotest.Equal(it, "none", left.Status)
			gotest.Zero(it, left.Duration)
			for _, leaf := range []string{"has_no_verdict", "takes_no_time"} {
				row := node(it, pkg.Nodes, "TestDeclaredTestSuite", "TestLeftOut", "the_stream_leaves_the_method_out", leaf)
				gotest.Equal(it, "none", row.Status)
				gotest.Zero(it, row.Duration)
			}
			terminal := spec(it, 0, "--input="+s.input, "--declared", declaredPkg)
			gotest.Regexp(it, `\n      \? has no verdict — NOT RUN\n`, terminal.Stdout)
			gotest.Regexp(it, `\n  LeftOut\n`, terminal.Stdout)
		})
		w.It("counts the behaviors the stream did not report as not run, and exits as without --declared", func(it *gotest.T) {
			doc := specJSON(it, 0, "--input="+s.input, "--declared", declaredPkg)
			gotest.Equal(it, 2, doc.Stats.NotRun)
			gotest.Equal(it, 3, doc.Stats.Passed)
			gotest.Equal(it, 6, doc.Stats.Behaviors)
			gotest.Equal(it, doc.Stats, *doc.Packages[0].Stats)
			terminal := spec(it, 0, "--input="+s.input, "--declared", declaredPkg)
			gotest.Regexp(it, `: 3 passed, 1 skipped, 2 not run\n$`, terminal.Stdout)
			markdown := spec(it, 0, "--format=md", "--input="+s.input, "--declared", declaredPkg)
			gotest.Contains(it, markdown.Stdout, ": 3 passed, 0 failed, 1 skipped, 2 not run.")
			without := specJSON(it, 0, "--input="+s.input)
			gotest.Zero(it, without.Stats.NotRun)
			spec(it, 0, "--render-only", "--input="+s.input, "--declared", declaredPkg)
		})
		w.It("reads a method the X_ prefix excludes, and its rows, as skipped rather than not run", func(it *gotest.T) {
			doc := specJSON(it, 0, "--input="+s.input, "--declared", declaredPkg)
			pkg := doc.Packages[0]
			gotest.Equal(it, "skip", node(it, pkg.Nodes, "TestDeclaredTestSuite", "X_TestParked").Status)
			gotest.Equal(it, "skip", node(it, pkg.Nodes, "TestDeclaredTestSuite", "X_TestParked", "the_method_is_parked", "is_skipped_with_it").Status)
			gotest.Equal(it, 1, doc.Stats.Skipped)
		})
	})
}

func (s *SpecDeclaredTestSuite) TestRowsOnlyARunCanName(t *gotest.T) {
	t.When("a stream holds the rows of an Each whose table is not a literal, rendered with --declared", func(w *gotest.T) {
		w.It("shows the rows under their method, which is marked incomplete", func(it *gotest.T) {
			doc := specJSON(it, 0, "--input="+s.input, "--declared", declaredPkg)
			method := node(it, doc.Packages[0].Nodes, "TestDeclaredTestSuite", "TestRanRowsFromAVariable")
			gotest.True(it, method.Incomplete)
			var rows []string
			for _, row := range method.Children {
				gotest.Equal(it, "pass", row.Status)
				rows = append(rows, row.Name)
			}
			gotest.Equal(it, []string{"first", "second"}, rows)
		})
	})
}

// discovered is what gotest discover says of a package's suites.
type discovered struct {
	Packages []struct {
		Dir    string `json:"dir"`
		Suites []struct {
			Name    string `json:"name"`
			File    string `json:"file"`
			Line    int    `json:"line"`
			Methods []struct {
				Name      string               `json:"name"`
				File      string               `json:"file"`
				Line      int                  `json:"line"`
				Behaviors []discoveredBehavior `json:"behaviors"`
			} `json:"methods"`
		} `json:"suites"`
	} `json:"packages"`
}

type discoveredBehavior struct {
	Name     string               `json:"name"`
	Line     int                  `json:"line"`
	Children []discoveredBehavior `json:"children"`
}

// position is a node's place in the source, as one document gives it.
type position struct {
	File string
	Line int
}

func (s *SpecDeclaredTestSuite) TestWhereTheSourceDeclaresANode(t *gotest.T) {
	t.When("a package's specification is rendered as JSON with --static or with --declared", func(w *gotest.T) {
		w.It("gives the package its dir and each node the source declares the file and line gotest discover gives it, and a node only the stream holds neither", func(it *gotest.T) {
			run, err := testkit.RunCLI(it.Context(), ".", "discover", declaredPkg)
			gotest.NoError(it, err)
			gotest.Equal(it, 0, run.Exit, "stderr: %s", run.Stderr)
			var disc discovered
			gotest.NoError(it, json.Unmarshal([]byte(run.Stdout), &disc))
			gotest.Len(it, disc.Packages, 1)
			want := map[string]position{}
			var behaviors func(prefix, file string, bs []discoveredBehavior)
			behaviors = func(prefix, file string, bs []discoveredBehavior) {
				for _, b := range bs {
					want[prefix+"/"+b.Name] = position{file, b.Line}
					behaviors(prefix+"/"+b.Name, file, b.Children)
				}
			}
			for _, suite := range disc.Packages[0].Suites {
				path := "Test" + suite.Name
				want[path] = position{suite.File, suite.Line}
				for _, m := range suite.Methods {
					want[path+"/"+m.Name] = position{m.File, m.Line}
					behaviors(path+"/"+m.Name, m.File, m.Behaviors)
				}
			}
			gotest.Len(it, want, 12)

			for sub, r := range gotest.Each(it, []struct {
				Desc string
				args []string
			}{
				{Desc: "read with --static", args: []string{"--static", declaredPkg}},
				{Desc: "laid under a stream with --declared", args: []string{"--input=" + s.input, "--declared", declaredPkg}},
			}) {
				doc := specJSON(sub, 0, r.args...)
				gotest.Len(sub, doc.Packages, 1)
				gotest.Equal(sub, disc.Packages[0].Dir, doc.Packages[0].Dir)
				got := map[string]position{}
				var walk func(prefix string, nodes []specNode)
				walk = func(prefix string, nodes []specNode) {
					for _, n := range nodes {
						path := n.Name
						if prefix != "" {
							path = prefix + "/" + n.Name
						}
						if n.File != "" || n.Line != 0 {
							got[path] = position{n.File, n.Line}
						}
						walk(path, n.Children)
					}
				}
				walk("", doc.Packages[0].Nodes)
				gotest.Equal(sub, want, got)
			}
			// The Each rows only the run names carry no position.
			doc := specJSON(it, 0, "--input="+s.input, "--declared", declaredPkg)
			row := node(it, doc.Packages[0].Nodes, "TestDeclaredTestSuite", "TestRanRowsFromAVariable", "first")
			gotest.Zero(it, row.File)
			gotest.Zero(it, row.Line)
		})
	})
}
