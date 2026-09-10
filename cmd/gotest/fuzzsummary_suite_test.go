package main_test

import (
	"bytes"
	"time"

	main "github.com/mvrahden/go-test/cmd/gotest"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// FuzzSummaryTestSuite pins the session's closing line and its step-summary
// markdown.
type FuzzSummaryTestSuite struct{}

func (s *FuzzSummaryTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func sessionResult() gotestrunner.FuzzRunResult {
	return gotestrunner.FuzzRunResult{Outcomes: []gotestrunner.FuzzTargetOutcome{
		{Func: "FuzzCodecTestSuite_FuzzRoundTrip", Execs: 1_407_280, Interesting: 3},
		{Func: "FuzzCodecTestSuite_FuzzDecode", ExitCode: 1, NewCrashers: []string{"f5000"}, Execs: 12_000},
		{Func: "FuzzCodecTestSuite_FuzzTrim", Skipped: true},
	}}
}

func quietResult() gotestrunner.FuzzRunResult {
	return gotestrunner.FuzzRunResult{Outcomes: []gotestrunner.FuzzTargetOutcome{{Func: "FuzzA", Execs: 5}}}
}

func (s *FuzzSummaryTestSuite) TestSessionLine(t *gotest.T) {
	t.It("counts execs, interesting inputs, crashers and targets never started", func(it *gotest.T) {
		line := main.ExportFuzzSessionLine(sessionResult(), 20300*time.Millisecond)
		gotest.Equal(it, "fuzzed 3 targets in 20.3s: 1,419,280 execs, 3 new interesting inputs, 1 new crasher (FuzzCodecTestSuite_FuzzDecode), 1 target never started", line)
	})

	t.It("says no crashers for a quiet session", func(it *gotest.T) {
		gotest.Equal(it, "fuzzed 1 target in 1.0s: 5 execs, 0 new interesting inputs, no crashers", main.ExportFuzzSessionLine(quietResult(), time.Second))
	})
}

func (s *FuzzSummaryTestSuite) TestSessionMarkdown(t *gotest.T) {
	t.It("renders one table row per target and points at triage", func(it *gotest.T) {
		var buf bytes.Buffer
		main.ExportRenderFuzzSessionMarkdown(&buf, sessionResult(), 20300*time.Millisecond)
		out := buf.String()
		gotest.Contains(it, out, "### Fuzzed 3 targets in 20.3s — 1 new crasher")
		gotest.Contains(it, out, "| Target | execs | new interesting | outcome |")
		gotest.Contains(it, out, "| FuzzCodecTestSuite_FuzzRoundTrip | 1,407,280 | 3 | no failures |")
		gotest.Contains(it, out, "| FuzzCodecTestSuite_FuzzDecode | 12,000 | 0 | 1 new crasher: f5000 |")
		gotest.Contains(it, out, "| FuzzCodecTestSuite_FuzzTrim | — | — | never started |")
		gotest.Contains(it, out, "`gotest fuzz triage`")
	})

	t.It("omits the triage hint when nothing crashed", func(it *gotest.T) {
		var buf bytes.Buffer
		main.ExportRenderFuzzSessionMarkdown(&buf, quietResult(), time.Second)
		gotest.Contains(it, buf.String(), "### Fuzzed 1 target in 1.0s — no crashers")
		gotest.NotContains(it, buf.String(), "triage")
	})
}
