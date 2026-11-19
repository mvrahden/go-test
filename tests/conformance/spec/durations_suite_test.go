package spec_test

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// SpecDurationsTestSuite: suites and methods carry their duration however
// short, Whens and Its from a second up, rows without a verdict none. The
// stream holds a 128ms suite, method, When and It, the same at 1.2s, and a
// method that never reported.
type SpecDurationsTestSuite struct{}

func (s *SpecDurationsTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second}
}

const durationsStream = "--input=../testdata/specstreams/durations.jsonl"

func (s *SpecDurationsTestSuite) TestMarkdown(t *gotest.T) {
	md := spec(t, 1, "--format=md", durationsStream).Stdout
	t.When("the Markdown spec of suites, methods, Whens and Its that took 128ms and 1.2s", func(w *gotest.T) {
		w.It("each suite heading and each method item carries its duration", func(it *gotest.T) {
			gotest.Contains(it, md, "\n#### Fast — 1 passed (128ms)\n")
			gotest.Contains(it, md, "\n- **Quick** (128ms)\n")
			gotest.Contains(it, md, "\n#### Slow — FAILED — 1 passed, 1 failed, 1 not run (1.2s)\n")
			gotest.Contains(it, md, "\n- **Waits** (1.2s)\n")
		})
		w.It("a When and an It of 1.2s carry theirs", func(it *gotest.T) {
			gotest.Contains(it, md, "\n  - when it waits (1.2s)\n    - PASS takes its time (1.2s)\n")
		})
		w.It("a When and an It of 128ms carry none", func(it *gotest.T) {
			gotest.Contains(it, md, "\n  - when it hurries\n    - PASS is done soon\n")
		})
		w.It("a method with no verdict carries none", func(it *gotest.T) {
			gotest.Contains(it, md, "\n- **NOT RUN** **Never**\n")
		})
	})
}

func (s *SpecDurationsTestSuite) TestTerminal(t *gotest.T) {
	out := spec(t, 1, durationsStream).Stdout
	t.When("the terminal spec of suites, methods, Whens and Its that took 128ms and 1.2s", func(w *gotest.T) {
		w.It("each suite line and each method line carries its duration", func(it *gotest.T) {
			gotest.Contains(it, out, "\nFast — 1 passed (128ms)\n")
			gotest.Contains(it, out, "\n  Quick (128ms)\n")
			gotest.Contains(it, out, "\nSlow ✗ — 1 passed, 1 failed, 1 not run (1.2s)\n")
			gotest.Contains(it, out, "\n  Waits (1.2s)\n")
		})
		w.It("a When and an It of 1.2s carry theirs", func(it *gotest.T) {
			gotest.Contains(it, out, "\n    when it waits (1.2s)\n      ✓ takes its time (1.2s)\n")
		})
		w.It("a When and an It of 128ms carry none", func(it *gotest.T) {
			gotest.Contains(it, out, "\n    when it hurries\n      ✓ is done soon\n")
		})
		w.It("a method with no verdict carries none", func(it *gotest.T) {
			gotest.Contains(it, out, "\n  ○ Never — NOT RUN\n")
		})
	})
}
