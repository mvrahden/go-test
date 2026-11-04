package spec_test

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// SpecVerdictsTestSuite: a branch and its package carry the verdict of a
// failure beneath them, whichever run each line came from.
type SpecVerdictsTestSuite struct{}

func (s *SpecVerdictsTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 45 * time.Second}
}

const (
	// composedStream holds a suite's pass line above a method's fail line,
	// as two runs composed it; failedStream is the same run with the suite's
	// and the package's lines saying fail, as go test writes it.
	composedStream = "--input=../testdata/specstreams/composed.jsonl"
	failedStream   = "--input=../testdata/specstreams/failed.jsonl"
)

func (s *SpecVerdictsTestSuite) TestAFailureBelowFailsTheBranch(t *gotest.T) {
	t.When("a stream holds a suite's pass line above a fail line of one of its methods", func(w *gotest.T) {
		w.It("renders the suite and its package as failed in the terminal, Markdown and JSON formats, as when their own lines say fail", func(it *gotest.T) {
			for sub, r := range gotest.Each(it, []struct {
				Desc   string
				format string
			}{
				{Desc: "for a terminal", format: "terminal"},
				{Desc: "as Markdown", format: "md"},
			}) {
				composed := spec(sub, 1, "--format="+r.format, composedStream)
				failed := spec(sub, 1, "--format="+r.format, failedStream)
				gotest.NotEmpty(sub, failed.Stdout)
				gotest.Equal(sub, failed.Stdout, composed.Stdout)
			}
			// The JSON carries each line's own output, so the two documents
			// agree on everything else.
			composed := specJSON(it, 1, composedStream)
			gotest.Equal(it, specJSON(it, 1, failedStream), composed)
			gotest.Len(it, composed.Packages, 1)
			gotest.Equal(it, "fail", composed.Packages[0].Status)
			gotest.Equal(it, "fail", node(it, composed.Packages[0].Nodes, "TestAccountTestSuite").Status)
		})
		w.It("counts one failure and exits 1, or 0 with --render-only", func(it *gotest.T) {
			doc := specJSON(it, 1, composedStream)
			gotest.Equal(it, 1, doc.Stats.Failed)
			gotest.Equal(it, 1, doc.Stats.Passed)
			terminal := spec(it, 1, composedStream)
			gotest.Regexp(it, `1 passed, 1 failed\n$`, terminal.Stdout)
			spec(it, 0, "--render-only", composedStream)
		})
	})
}
