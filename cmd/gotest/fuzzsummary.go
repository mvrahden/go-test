package main

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/gotestrunner"
)

// fuzzSessionLine is the one closing line of a fuzz session: what ran, how
// much, and whether anything was found.
func fuzzSessionLine(res gotestrunner.FuzzRunResult, wall time.Duration) string {
	var execs int64
	interesting, skipped := 0, 0
	var crashed []string
	for _, o := range res.Outcomes {
		execs += o.Execs
		interesting += o.Interesting
		if o.Skipped {
			skipped++
		}
		if len(o.NewCrashers) > 0 {
			crashed = append(crashed, o.Func)
		}
	}
	n := len(res.Outcomes)
	parts := []string{
		fmt.Sprintf("%s execs", groupDigits(execs)),
		fmt.Sprintf("%d new interesting input%s", interesting, plural(interesting)),
	}
	switch {
	case len(crashed) > 0:
		total := 0
		for _, o := range res.Outcomes {
			total += len(o.NewCrashers)
		}
		parts = append(parts, fmt.Sprintf("%d new crasher%s (%s)", total, plural(total), strings.Join(crashed, ", ")))
	default:
		parts = append(parts, "no crashers")
	}
	if skipped > 0 {
		parts = append(parts, fmt.Sprintf("%d target%s never started", skipped, plural(skipped)))
	}
	return fmt.Sprintf("fuzzed %d target%s in %s: %s", n, plural(n), fuzzWall(wall), strings.Join(parts, ", "))
}

// renderFuzzSessionMarkdown is the session as a GitHub step summary: the
// headline, one row per target, and the next step when something was found.
func renderFuzzSessionMarkdown(w io.Writer, res gotestrunner.FuzzRunResult, wall time.Duration) {
	crashers := 0
	for _, o := range res.Outcomes {
		crashers += len(o.NewCrashers)
	}
	verdict := "no crashers"
	if crashers > 0 {
		verdict = fmt.Sprintf("%d new crasher%s", crashers, plural(crashers))
	}
	n := len(res.Outcomes)
	fmt.Fprintf(w, "### Fuzzed %d target%s in %s — %s\n\n", n, plural(n), fuzzWall(wall), verdict)
	fmt.Fprintln(w, "| Target | execs | new interesting | outcome |")
	fmt.Fprintln(w, "|---|---|---|---|")
	for _, o := range res.Outcomes {
		if o.Skipped {
			fmt.Fprintf(w, "| %s | — | — | never started |\n", o.Func)
			continue
		}
		fmt.Fprintf(w, "| %s | %s | %d | %s |\n", o.Func, groupDigits(o.Execs), o.Interesting, fuzzOutcomeText(o))
	}
	if crashers > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Inspect with `gotest fuzz triage`, then `gotest fuzz promote` to keep each crasher as a typed seed.")
	}
}

func fuzzOutcomeText(o gotestrunner.FuzzTargetOutcome) string { //nolint:gocritic // hugeParam: stable API
	switch {
	case len(o.NewCrashers) > 0:
		return fmt.Sprintf("%d new crasher%s: %s", len(o.NewCrashers), plural(len(o.NewCrashers)), strings.Join(o.NewCrashers, ", "))
	case o.EffectiveExitCode() == 2:
		return "could not run"
	case o.EffectiveExitCode() == 1:
		return "failing seed or corpus entry"
	case o.Canceled:
		return "stopped early, no failures"
	default:
		return "no failures"
	}
}

func fuzzWall(d time.Duration) string { return fmt.Sprintf("%.1fs", d.Seconds()) }

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// groupDigits renders n with thousands separators: 1407280 -> 1,407,280.
func groupDigits(n int64) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	head := len(s) % 3
	if head > 0 {
		b.WriteString(s[:head])
	}
	for i := head; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}
