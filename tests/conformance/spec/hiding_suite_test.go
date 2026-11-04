package spec_test

import (
	"strings"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// SpecHidingTestSuite: --hide leaves rows out of a rendering by their own
// status and counts what it would have counted without it.
type SpecHidingTestSuite struct{}

func (s *SpecHidingTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 45 * time.Second}
}

const (
	// hidingStream fails one of Account's two methods and Boot's AfterAll,
	// whose two methods pass; greenStream is the same run all passing.
	hidingStream = "--input=../testdata/specstreams/hiding.jsonl"
	greenStream  = "--input=../testdata/specstreams/green.jsonl"
)

// names lists every node's name, depth first.
func names(nodes []specNode) []string {
	var out []string
	for _, n := range nodes {
		out = append(out, n.Name)
		out = append(out, names(n.Children)...)
	}
	return out
}

func (s *SpecHidingTestSuite) TestHidingWhatPassed(t *gotest.T) {
	t.When("a failing stream is rendered with --hide=pass", func(w *gotest.T) {
		w.It("keeps the failing branch, its failing leaf and a suite that failed in its own hooks, and leaves out the rows that passed", func(it *gotest.T) {
			doc := specJSON(it, 1, "--hide=pass", hidingStream)
			gotest.Len(it, doc.Packages, 1)
			gotest.Equal(it, []string{"TestAccountTestSuite", "TestOpen", "TestBootTestSuite"}, names(doc.Packages[0].Nodes))
			terminal := spec(it, 1, "--hide=pass", hidingStream)
			gotest.Contains(it, terminal.Stdout, "✗ Open")
			gotest.Contains(it, terminal.Stdout, "could not release the database")
			gotest.NotContains(it, terminal.Stdout, "✓")
			markdown := spec(it, 1, "--format=md", "--hide=pass", hidingStream)
			gotest.Contains(it, markdown.Stdout, "| Open | FAIL |")
			gotest.Contains(it, markdown.Stdout, "## Boot — FAILED")
			gotest.NotContains(it, markdown.Stdout, "PASS")
		})
		w.It("counts every behavior that passed in the trailer and in its package's stats, as without --hide", func(it *gotest.T) {
			hidden := specJSON(it, 1, "--hide=pass", hidingStream)
			whole := specJSON(it, 1, "--hide=", hidingStream)
			gotest.Equal(it, 3, whole.Stats.Passed)
			gotest.Equal(it, whole.Stats, hidden.Stats)
			gotest.NotZero(it, hidden.Packages[0].Stats)
			gotest.Equal(it, *whole.Packages[0].Stats, *hidden.Packages[0].Stats)
			gotest.Equal(it, whole.Stats, *hidden.Packages[0].Stats)
			terminal := spec(it, 1, "--hide=pass", hidingStream)
			gotest.Regexp(it, `\n2 suites, 5 behaviors: 3 passed, 2 failed\n$`, terminal.Stdout)
			markdown := spec(it, 1, "--format=md", "--hide=pass", hidingStream)
			gotest.Contains(it, markdown.Stdout, "2 suites, 5 behaviors: 3 passed, 2 failed, 0 skipped.")
		})
	})
}

func (s *SpecHidingTestSuite) TestHidingAGreenRun(t *gotest.T) {
	t.When("a stream in which everything passed is rendered with --hide=pass", func(w *gotest.T) {
		w.It("renders the trailer and no row", func(it *gotest.T) {
			terminal := spec(it, 0, "--hide=pass", greenStream)
			gotest.Equal(it, "\n3 suites, 5 behaviors: 5 passed\n", terminal.Stdout)
			markdown := spec(it, 0, "--format=md", "--hide=pass", greenStream)
			gotest.Equal(it, "# Behavior Specification\n\n3 suites, 5 behaviors: 5 passed, 0 failed, 0 skipped.\n\n", markdown.Stdout)
		})
		w.It("keeps every package in the JSON document with all of its stats and none of its rows", func(it *gotest.T) {
			hidden := specJSON(it, 0, "--hide=pass", greenStream)
			whole := specJSON(it, 0, "--hide=", greenStream)
			gotest.Equal(it, 5, hidden.Stats.Passed)
			gotest.Len(it, hidden.Packages, 2)
			gotest.Len(it, whole.Packages, 2)
			for i, pkg := range hidden.Packages {
				gotest.Equal(it, whole.Packages[i].Path, pkg.Path)
				gotest.Empty(it, pkg.Nodes)
				gotest.NotZero(it, pkg.Stats)
				gotest.Equal(it, *whole.Packages[i].Stats, *pkg.Stats)
				gotest.NotZero(it, pkg.Stats.Passed)
			}
		})
	})
}

func (s *SpecHidingTestSuite) TestHidingAPackagesOwnFailure(t *gotest.T) {
	const broken = "--input=../testdata/specstreams/buildfail.jsonl"
	t.When("a stream whose package failed to build is rendered with --hide=fail", func(w *gotest.T) {
		w.It("leaves the package's failure and its output out of every format, its stats still counting it", func(it *gotest.T) {
			shown := spec(it, 0, "--render-only", broken)
			gotest.Contains(it, shown.Stdout, "syntax error")
			terminal := spec(it, 0, "--render-only", "--hide=fail", broken)
			gotest.Equal(it, "\n0 suites: 1 failed package\n", terminal.Stdout)
			markdown := spec(it, 0, "--render-only", "--format=md", "--hide=fail", broken)
			gotest.NotContains(it, markdown.Stdout, "FAILED")
			gotest.NotContains(it, markdown.Stdout, "syntax error")
			run := spec(it, 0, "--render-only", "--format=json", "--hide=fail", broken)
			gotest.NotContains(it, run.Stdout, "syntax error")
			doc := specJSON(it, 0, "--render-only", "--hide=fail", broken)
			gotest.Len(it, doc.Packages, 1)
			gotest.Empty(it, doc.Packages[0].Output)
			gotest.Equal(it, 1, doc.Packages[0].Stats.FailedPackages)
			gotest.Equal(it, 1, doc.Stats.FailedPackages)
		})
	})
}

func (s *SpecHidingTestSuite) TestAViewThatCannotApply(t *gotest.T) {
	t.When("--hide is given with --static or with a status it does not know, or --declared without --input or with --static", func(w *gotest.T) {
		w.It("refuses with exit 2, naming what it needs or the statuses it accepts", func(it *gotest.T) {
			for sub, r := range gotest.Each(it, []struct {
				Desc   string
				args   []string
				stderr string
			}{
				{Desc: "--hide under --static", args: []string{"--static", "--hide=pass", "../testdata/declared/"}, stderr: "--static has none"},
				{Desc: "an unknown status", args: []string{"--hide=green", hidingStream}, stderr: "use pass, fail, skip or none"},
				{Desc: "--declared without --input", args: []string{"--declared", "../testdata/declared/"}, stderr: "--declared requires --input"},
				{Desc: "--declared with --static", args: []string{"--static", "--declared", "../testdata/declared/"}, stderr: "--static and --declared are mutually exclusive"},
			}) {
				run := spec(sub, 2, r.args...)
				gotest.Empty(sub, strings.TrimSpace(run.Stdout))
				gotest.Contains(sub, run.Stderr, r.stderr)
			}
		})
	})
}
