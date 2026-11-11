package spec_test

import (
	"os/exec"
	"strings"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// SpecReportTestSuite: the Markdown spec a person reads, over a stream of a
// suite of both test packages with a failure, a behavior that started and
// reported no verdict, a 300ms and a 1.2s method.
type SpecReportTestSuite struct{}

func (s *SpecReportTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 45 * time.Second}
}

const reportStream = "--input=../testdata/specstreams/report.jsonl"

// headCommit is the checkout's short commit, "" outside one.
func headCommit() string {
	out, err := exec.Command("git", "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func (s *SpecReportTestSuite) TestABehaviorWithoutAVerdict(t *gotest.T) {
	t.When("a behavior started and reported no verdict", func(w *gotest.T) {
		w.It("counts as not run", func(it *gotest.T) {
			doc := specJSON(it, 1, reportStream)
			gotest.Equal(it, 1, doc.Stats.NotRun)
			gotest.Equal(it, "none", node(it, doc.Packages[0].Nodes[1:], "TestUserRepositoryTestSuite", "TestDeleteUser").Status)
			md := spec(it, 1, "--format=md", reportStream)
			gotest.Contains(it, md.Stdout, "\n- **NOT RUN** **DeleteUser**\n")
		})
		w.It("shows no duration", func(it *gotest.T) {
			doc := specJSON(it, 1, reportStream)
			gotest.Zero(it, node(it, doc.Packages[0].Nodes[1:], "TestUserRepositoryTestSuite", "TestDeleteUser").Duration)
			md := spec(it, 1, "--format=md", reportStream)
			gotest.Regexp(it, `\n- \*\*NOT RUN\*\* \*\*DeleteUser\*\*\n`, md.Stdout)
			terminal := spec(it, 1, reportStream)
			gotest.Regexp(it, `\n *\S+ DeleteUser — NOT RUN\n`, terminal.Stdout)
		})
		w.It("the counts add up to the behaviors", func(it *gotest.T) {
			doc := specJSON(it, 1, reportStream)
			gotest.Equal(it, doc.Stats.Behaviors, doc.Stats.Passed+doc.Stats.Failed+doc.Stats.Skipped+doc.Stats.NotRun)
			md := spec(it, 1, "--format=md", reportStream)
			gotest.Contains(it, md.Stdout, "\n2 suites, 5 behaviors: 3 passed, 1 failed, 1 not run.\n")
		})
	})
}

func (s *SpecReportTestSuite) TestAReportAPersonReads(t *gotest.T) {
	md := spec(t, 1, "--format=md", reportStream).Stdout
	packages := strings.Index(md, "\n### ")
	gotest.NotEqual(t, -1, packages, "no package heading:\n%s", md)
	t.When("the Markdown spec of a run with failures and rows not run", func(w *gotest.T) {
		w.It("the header says the time range of the runs and the commit", func(it *gotest.T) {
			header := strings.SplitN(md, "\n", 6)[4]
			want := "Runs from 2026-11-09 18:02 to 18:25 UTC"
			if commit := headCommit(); commit != "" {
				gotest.Regexp(it, "^"+want+" · ([^ ]+ @ )?"+commit+"(, uncommitted changes)?$", header)
				return
			}
			gotest.Equal(it, want, header)
		})
		w.It("lists every failure with its path and message before the packages", func(it *gotest.T) {
			want := "\n## Needs attention\n\n- **FAIL** example.com/report › UserRepository › CreateUser › when a user is created › has one entry\n" +
				"  ```text\n  suite_ext_test.go:28: Len: expected 1, got 2\n  ```\n"
			at := strings.Index(md, want)
			gotest.NotEqual(it, -1, at, "%s", md)
			gotest.Less(it, at, packages)
		})
		w.It("lists every row not run with its path", func(it *gotest.T) {
			at := strings.Index(md, "\n- **NOT RUN** example.com/report › UserRepository › DeleteUser\n")
			gotest.NotEqual(it, -1, at, "%s", md)
			gotest.Less(it, at, packages)
		})
		w.It("every package and suite heading carries its counts", func(it *gotest.T) {
			for _, heading := range []string{
				"\n### example.com/report — 3 passed, 1 failed, 1 not run\n",
				"\n#### UserRepository — 2 passed\n",
				"\n#### UserRepository — EXTERNAL — 1 passed, 1 failed, 1 not run\n",
			} {
				gotest.Contains(it, md, heading)
			}
		})
		w.It("an external suite's heading is marked EXTERNAL", func(it *gotest.T) {
			gotest.Equal(it, 1, strings.Count(md, " — EXTERNAL"))
			gotest.Contains(it, md, "\n#### UserRepository — EXTERNAL — ")
		})
		w.It("no duration under a second is shown", func(it *gotest.T) {
			gotest.Contains(it, md, "\n- PASS **Quick**\n")
			gotest.Contains(it, md, "\n- PASS **Ping** (1.2s)\n")
			gotest.NotContains(it, md, "ms)")
		})
	})

	t.When("the Markdown spec hides rows", func(w *gotest.T) {
		w.It("the header names what is hidden", func(it *gotest.T) {
			hidden := spec(it, 1, "--format=md", "--hide=pass,skip", reportStream).Stdout
			gotest.Regexp(it, "\n\n[^\n]* · hiding passed and skipped rows\n\n", hidden)
			gotest.NotContains(it, md, "hiding")
		})
	})
}
