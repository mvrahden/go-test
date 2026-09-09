package main //nolint:stdlib-test

import (
	"bytes"
	"testing"
	"time"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

func sessionResult() gotestrunner.FuzzRunResult {
	return gotestrunner.FuzzRunResult{Outcomes: []gotestrunner.FuzzTargetOutcome{
		{Func: "FuzzCodecTestSuite_FuzzRoundTrip", Execs: 1_407_280, Interesting: 3},
		{Func: "FuzzCodecTestSuite_FuzzDecode", ExitCode: 1, NewCrashers: []string{"f5000"}, Execs: 12_000},
		{Func: "FuzzCodecTestSuite_FuzzTrim", Skipped: true},
	}}
}

func TestFuzzSessionLine(t *testing.T) {
	line := fuzzSessionLine(sessionResult(), 20300*time.Millisecond)
	gotest.Equal(t, "fuzzed 3 targets in 20.3s: 1,419,280 execs, 3 new interesting inputs, 1 new crasher (FuzzCodecTestSuite_FuzzDecode), 1 target never started", line)
}

func TestFuzzSessionLine_Quiet(t *testing.T) {
	res := gotestrunner.FuzzRunResult{Outcomes: []gotestrunner.FuzzTargetOutcome{{Func: "FuzzA", Execs: 5}}}
	gotest.Equal(t, "fuzzed 1 target in 1.0s: 5 execs, 0 new interesting inputs, no crashers", fuzzSessionLine(res, time.Second))
}

func TestRenderFuzzSessionMarkdown(t *testing.T) {
	var buf bytes.Buffer
	renderFuzzSessionMarkdown(&buf, sessionResult(), 20300*time.Millisecond)
	out := buf.String()
	gotest.Contains(t, out, "### Fuzzed 3 targets in 20.3s — 1 new crasher")
	gotest.Contains(t, out, "| Target | execs | new interesting | outcome |")
	gotest.Contains(t, out, "| FuzzCodecTestSuite_FuzzRoundTrip | 1,407,280 | 3 | no failures |")
	gotest.Contains(t, out, "| FuzzCodecTestSuite_FuzzDecode | 12,000 | 0 | 1 new crasher: f5000 |")
	gotest.Contains(t, out, "| FuzzCodecTestSuite_FuzzTrim | — | — | never started |")
	gotest.Contains(t, out, "`gotest fuzz triage`")
}

func TestRenderFuzzSessionMarkdown_NoCrashers(t *testing.T) {
	var buf bytes.Buffer
	renderFuzzSessionMarkdown(&buf, gotestrunner.FuzzRunResult{Outcomes: []gotestrunner.FuzzTargetOutcome{{Func: "FuzzA", Execs: 5}}}, time.Second)
	gotest.Contains(t, buf.String(), "### Fuzzed 1 target in 1.0s — no crashers")
	gotest.NotContains(t, buf.String(), "triage")
}
