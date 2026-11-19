package docref

import (
	"fmt"
	"strings"
)

// FlagDoc is the one description of one of gotest's own flags.
type FlagDoc struct {
	Name  string // "--min"
	Value string // "<pct>", or "" for a flag that takes none
	Doc   string // Markdown: `code` spans only
}

// FlagDocs describes each of gotest's own flags, in the reference's order.
var FlagDocs = []FlagDoc{
	{"--debug", "", "Keep the run's overlay and work dir for inspection; a later run removes them after 7 days"},
	{"--ci", "", "CI mode: fail on `F_` prefixes, snapshot read-only"},
	{"--no-cache", "", "Bypass the test binary cache and link afresh (generation is not cached)"},
	{"--spec", "", "Render the spec view instead of the default output (default and watch modes)"},
	{"--update-snapshots", "", "Regenerate snapshot files"},
	{"--format", "<fmt>", "Output format for `spec`/`summary` (terminal, md/markdown, json)"},
	{"--output", "<path>", "Write formatted output to file"},
	{"--no-color", "", "Strip ANSI codes from terminal output"},
	{"--min", "<pct>", "Fail if coverage is below threshold (enables `-coverprofile`)"},
	{"--setup-timeout", "<dur>", "Flat budget for shared fixture setup (default: the fixtures' declared budgets, 30s until the first reports; 0 disables)"},
	{"--timeout", "<dur>", "Global pipeline deadline (default: 15m; 0 disables); refused by `fuzz`, whose deadline follows `--for`"},
	{"--debounce", "<dur>", "Debounce interval for watch mode (default 200ms)"},
	{"--parallel", "<n>", "Total concurrent test method budget (default: 2×GOMAXPROCS, auto-halved for -race/-msan/-asan)"},
	{"--compile-parallel", "<n>", "Concurrent compilation processes (default: NumCPU, auto-halved for -race/-msan/-asan)"},
	{"--group", "<name>", "Run only the suites of this `ExclusiveGroup` (repeatable; default runs, `spec`, `summary`); a name no suite declares exits 2 before any build"},
	{"--skip-group", "<name>", "Leave the suites of this `ExclusiveGroup` out (repeatable)"},
	{"--input", "<path>", "Replay a saved `go test -json` stream in `spec`/`summary` (`-` reads stdin); exit 2 when it holds no test events, a package or failed build has no verdict, or a package failed to build"},
	{"--events-file", "<path>", "With `-json`, or in `summary`, also write the run's `go test -json` event stream to the file as `-json` prints it (a suite's events when its process ends), making its directories and replacing a file there; `summary --input` reads it back, and a text run refuses it (exit 2)"},
	{"--control", "stdin", "Read `stop` and `force` lines on standard input, a first and a second interrupt, counted with the signals, the input's end a stop when none is under way; taken by every command that stops on an interrupt, and without it standard input is never read"},
	{"--github", "", "Emit GitHub annotations and step summary (auto-enabled in GitHub Actions)"},
	{"--coverage", "<path>", "Coverage profile path for the `summary` subcommand"},
	{"--badge", "<path>", "Write a self-contained coverage badge SVG (`summary`); adds `-coverprofile` when neither the caller nor `--min` did"},
	{"--render-only", "", "With `--input`, exit 0 on a failing or incomplete stream: the code reports whether rendering succeeded, not whether the tests passed"},
	{"--stamp", "", "With `--input`, write the stream back with each test event placed as `gotest -json` places it, instead of rendering it (`spec`)"},
	{"--static", "", "Render the specification from source without running the suites (`spec`); nodes carry no verdict, and methods whose behaviors depend on runtime values are reported as incomplete on stderr"},
	{"--declared", "", "Lay the run's verdicts, or with `--input` a saved stream's, over the specification the packages' source declares (`spec`)"},
	{"--hide", "<statuses>", "Leave out of the `spec` rendering the rows whose status is one of `pass`, `fail`, `skip`, `none`, without changing what is counted"},
	{"--match", "<text>", "Keep of the `spec` rendering the behaviors whose label holds the text, and their groups; counts are those of what is kept"},
	{"--only", "<import path>[:<i>.<j>…]", "Keep of the `spec` rendering one package, or the node at that position among the shown children, with what is under it; counts are those of what is kept"},
	{"--bench", "", "Benchmark mode for `watch`: re-run benchmarks on change with ns/op deltas"},
	{"--save", "<path>", "Save a benchmark run as a JSON baseline (`bench`; a bare `--save=` uses `bench.baseline` from `.gotest.yml`, and saving to the configured baseline promotes the run without comparing it)"},
	{"--against", "<path>", "Compare a benchmark run against a saved baseline and print the delta table (`bench`; defaults to `bench.baseline`); a baseline from another machine or Go version warns and still compares"},
	{"--gate", "<pct>", "Fail (exit 1) if the worst significant benchmark regression exceeds the threshold (`bench`); needs `-count=4` or more"},
	{"--json", "", "Emit one versioned JSON report to stdout instead of human output (`bench`; for tooling)"},
	{"--for", "<dur>", "Approximate wall-clock fuzz budget for the session, split across targets (`fuzz`; default 1m or half of a shorter `--timeout`; `0` removes the budget)"},
	{"--target", "<name>", "Fuzz exactly one generated wrapper by name; an unmatched name lists the available ones (`fuzz`)"},
	{"--jobs", "<n>", "Max concurrent fuzz targets (`fuzz`; default: max(1, GOMAXPROCS/2))"},
	{"--no-harvest", "", "Disable table-test seed harvesting for this run"},
	{"--fuzz", "", "Generate fuzz round-trip skeletons (`scaffold`)"},
	{"--dry-run", "", "Print the unified diff of every edit instead of writing it (`migrate`)"},
	{"--dir", "<skills-dir>", "Skills directory to install into, default `.agents/skills` (`skill install`)"},
	{"--cache", "", "Remove every cached test binary no run is using (`clean`)"},
}

// FlagSections returns the renderer of flags/gotest.
func FlagSections() map[string]Renderer {
	spelled := func(f FlagDoc) string {
		if f.Value == "" {
			return f.Name
		}
		return f.Name + "=" + f.Value
	}
	return map[string]Renderer{"flags/gotest": formats(map[string]func() string{
		"table": func() string {
			var b strings.Builder
			b.WriteString("| Flag | Effect |\n|---|---|\n")
			for _, f := range FlagDocs {
				fmt.Fprintf(&b, "| `%s` | %s |\n", codeCell(spelled(f)), textCell(f.Doc))
			}
			return b.String()
		},
		"html": func() string {
			var b strings.Builder
			for _, f := range FlagDocs {
				fmt.Fprintf(&b, "<tr><td><code>%s</code></td><td>%s.</td></tr>\n", htmlText(spelled(f)), htmlText(f.Doc))
			}
			return b.String()
		},
	})}
}
