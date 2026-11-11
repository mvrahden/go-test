package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLIStreamsTestSuite: a machine-readable stdout holds nothing but the
// document asked for, color follows the terminal, and two output modes
// cannot be asked for at once.
type CLIStreamsTestSuite struct{}

func (s *CLIStreamsTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 45 * time.Second}
}

type streamRow struct {
	Desc string
	env  []string
	args []string
	exit int
}

func (s *CLIStreamsTestSuite) TestJSONOutputsStayPure(t *gotest.T) {
	t.When("a JSON document is asked for", func(w *gotest.T) {
		w.It("prints one JSON document with no color codes or annotations, inside GitHub Actions too", func(it *gotest.T) {
			for sub, row := range gotest.Each(it, []streamRow{
				{Desc: "summary --format json inside GitHub Actions carries no annotations", env: []string{"GITHUB_ACTIONS=true"}, args: []string{"summary", "--format", "json", "../testdata/failing/"}, exit: 1},
				{Desc: "summary --format json carries no color codes", args: []string{"summary", "--format", "json", "../testdata/passing/"}, exit: 0},
				{Desc: "spec --format json is one document", args: []string{"spec", "--format", "json", "../testdata/passing/"}, exit: 0},
			}) {
				run, err := testkit.RunCLIWithEnv(sub.Context(), ".", row.env, row.args...)
				gotest.NoError(sub, err)
				gotest.Equal(sub, row.exit, run.Exit, "stderr: %s", run.Stderr)
				gotest.NotContains(sub, run.Stdout, "\x1b[")
				gotest.NotContains(sub, run.Stdout, "::error")
				var doc any
				gotest.NoError(sub, json.Unmarshal([]byte(run.Stdout), &doc), "stdout is not one JSON document:\n%s", run.Stdout)
			}
		})
	})

	t.When("the go test -json stream is asked for", func(w *gotest.T) {
		w.It("holds JSON lines only, inside GitHub Actions too", func(it *gotest.T) {
			run, err := testkit.RunCLIWithEnv(it.Context(), ".", []string{"GITHUB_ACTIONS=true"}, "-json", "../testdata/failing/")
			gotest.NoError(it, err)
			gotest.Equal(it, 1, run.Exit, "stderr: %s", run.Stderr)
			for _, line := range strings.Split(strings.TrimSpace(run.Stdout), "\n") {
				gotest.Regexp(it, `^\{`, line)
			}
		})
	})
}

func (s *CLIStreamsTestSuite) TestRunLevelPackagesAreMarked(t *gotest.T) {
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/failingshared/")
	gotest.NoError(t, err)
	t.When("a -json run's shared fixture does not come up", func(w *gotest.T) {
		w.It("marks every event of the shared fixtures package RunLevel, its start line included, and no event of another package", func(it *gotest.T) {
			gotest.Equal(it, 1, run.Exit, "stderr: %s", run.Stderr)
			marked := map[string]int{}
			unmarked := map[string]int{}
			var actions []string
			for _, line := range strings.Split(strings.TrimSpace(run.Stdout), "\n") {
				var ev struct {
					Action   string
					Package  string
					RunLevel *bool
				}
				gotest.NoError(it, json.Unmarshal([]byte(line), &ev), "line: %s", line)
				if ev.RunLevel != nil {
					gotest.True(it, *ev.RunLevel, "line: %s", line)
					marked[ev.Package]++
				} else {
					unmarked[ev.Package]++
				}
				if ev.Package == "shared fixtures" {
					actions = append(actions, ev.Action)
				}
			}
			gotest.NotEmpty(it, actions)
			gotest.Equal(it, "start", actions[0])
			gotest.Equal(it, "fail", actions[len(actions)-1])
			gotest.Equal(it, map[string]int{"shared fixtures": len(actions)}, marked)
			gotest.NotEmpty(it, unmarked)
			gotest.Zero(it, unmarked["shared fixtures"])
		})
		w.It("spec's JSON document of that stream marks the shared fixtures package runLevel, and no other", func(it *gotest.T) {
			input := filepath.Join(it.TempDir(), "events.jsonl")
			gotest.NoError(it, os.WriteFile(input, []byte(run.Stdout), 0o600))
			doc, err := testkit.RunCLI(it.Context(), ".", "spec", "--input="+input, "--format=json", "--render-only")
			gotest.NoError(it, err)
			gotest.Equal(it, 0, doc.Exit, "stderr: %s", doc.Stderr)
			var spec struct {
				Packages []struct {
					Path     string
					RunLevel bool `json:"runLevel"`
				}
			}
			gotest.NoError(it, json.Unmarshal([]byte(doc.Stdout), &spec), "stdout: %s", doc.Stdout)
			marked := map[string]bool{}
			for _, p := range spec.Packages {
				marked[p.Path] = p.RunLevel
			}
			gotest.Len(it, marked, 2)
			gotest.Equal(it, map[string]bool{
				"shared fixtures": true,
				"github.com/mvrahden/go-test/tests/conformance/testdata/failingshared": false,
			}, marked)
		})
	})
}

// stampedEvent is an event of the stream with the object gotest adds.
type stampedEvent struct {
	Action string
	Test   string
	Gotest *struct {
		Subtest  string
		Repeat   int
		Instance int
	}
}

// goTestEvent mirrors the event go test's test2json writes, field for field.
type goTestEvent struct {
	Time        *time.Time `json:",omitempty"`
	Action      string
	Package     string   `json:",omitempty"`
	Test        string   `json:",omitempty"`
	Elapsed     *float64 `json:",omitempty"`
	Output      *string  `json:",omitempty"`
	OutputType  string   `json:",omitempty"`
	FailedBuild string   `json:",omitempty"`
	Key         string   `json:",omitempty"`
	Value       string   `json:",omitempty"`
	Path        string   `json:",omitempty"`
}

var gotestObject = regexp.MustCompile(`,"Gotest":\{[^{}]*\}\}$`)

func (s *CLIStreamsTestSuite) TestEventsSayWhereTheirTestSits(t *gotest.T) {
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/specslashes/")
	gotest.NoError(t, err)
	gotest.Equal(t, 0, run.Exit, "stderr: %s", run.Stderr)
	lines := strings.Split(strings.TrimSpace(run.Stdout), "\n")
	events := make([]stampedEvent, len(lines))
	for i, line := range lines {
		gotest.NoError(t, json.Unmarshal([]byte(line), &events[i]), "line: %s", line)
	}
	const twin = "TestTwinTestSuite"

	t.When("gotest -json runs a subtest", func(w *gotest.T) {
		w.It("its run event carries its own level in Gotest.Subtest", func(it *gotest.T) {
			subtests := map[string]string{}
			for _, ev := range events {
				if ev.Action != "run" {
					continue
				}
				gotest.NotZero(it, ev.Gotest, "run %s carries no Gotest object", ev.Test)
				gotest.Regexp(it, "(^|/)"+regexp.QuoteMeta(ev.Gotest.Subtest)+"$", ev.Test)
				subtests[ev.Test] = ev.Gotest.Subtest
			}
			gotest.Equal(it, "/usr/local/go/src", subtests["TestSlashesTestSuite/TestLabels/an_address_is_absolute//usr/local/go/src"])
			gotest.Equal(it, "example.com/m/pkg#01", subtests["TestSlashesTestSuite/TestRepeats/a_module_is_listed_twice/example.com/m/pkg#01"])
			gotest.Equal(it, "TestShared#01", subtests[twin+"/TestShared#01"])
			gotest.Equal(it, twin, subtests[twin])
		})
		w.It("every field go test wrote is kept as go test wrote it", func(it *gotest.T) {
			stamped := 0
			for i, line := range lines {
				if events[i].Test == "" {
					continue
				}
				if events[i].Gotest != nil {
					gotest.Regexp(it, gotestObject, line)
					line = gotestObject.ReplaceAllString(line, "}")
					stamped++
				}
				var ev goTestEvent
				dec := json.NewDecoder(strings.NewReader(line))
				dec.DisallowUnknownFields()
				gotest.NoError(it, dec.Decode(&ev), "line: %s", line)
				again, err := json.Marshal(ev)
				gotest.NoError(it, err)
				gotest.Equal(it, line, string(again))
			}
			gotest.NotZero(it, stamped)
		})
	})

	t.When("gotest -json runs a suite of both test packages", func(w *gotest.T) {
		w.It("every event of the external suite carries Gotest.Instance 2", func(it *gotest.T) {
			runs, external, internal := 0, 0, 0
			for _, ev := range events {
				if ev.Test != twin && !strings.HasPrefix(ev.Test, twin+"/") {
					continue
				}
				if ev.Test == twin && ev.Action == "run" {
					runs++
				}
				instance := 0
				if ev.Gotest != nil {
					instance = ev.Gotest.Instance
				}
				if runs == 2 {
					gotest.Equal(it, 2, instance, "%s %s", ev.Action, ev.Test)
					external++
				} else {
					gotest.Zero(it, instance, "%s %s", ev.Action, ev.Test)
					internal++
				}
			}
			gotest.Equal(it, 2, runs)
			gotest.NotZero(it, internal)
			gotest.NotZero(it, external)
		})
	})
}

func (s *CLIStreamsTestSuite) TestColorFollowsTheTerminal(t *gotest.T) {
	t.When("stdout is a pipe", func(w *gotest.T) {
		w.It("prints no color codes, even with --no-color=false", func(it *gotest.T) {
			for sub, row := range gotest.Each(it, []streamRow{
				{Desc: "summary prints no color codes", args: []string{"summary", "../testdata/passing/"}, exit: 0},
				{Desc: "summary --no-color=false still prints none", args: []string{"summary", "--no-color=false", "../testdata/passing/"}, exit: 0},
				{Desc: "spec prints no color codes", args: []string{"spec", "../testdata/passing/"}, exit: 0},
				{Desc: "NO_COLOR is honoured as well", env: []string{"NO_COLOR=1"}, args: []string{"summary", "../testdata/passing/"}, exit: 0},
			}) {
				run, err := testkit.RunCLIWithEnv(sub.Context(), ".", row.env, row.args...)
				gotest.NoError(sub, err)
				gotest.Equal(sub, row.exit, run.Exit, "stderr: %s", run.Stderr)
				gotest.NotContains(sub, run.Stdout, "\x1b[")
			}
		})
	})
}

func (s *CLIStreamsTestSuite) TestTwoOutputModesAreRefused(t *gotest.T) {
	t.When("two output modes are asked for at once", func(w *gotest.T) {
		w.It("refuses them, and an unknown --format, with exit 2", func(it *gotest.T) {
			for sub, row := range gotest.Each(it, []flagRow{
				{Desc: "--spec with -json", args: []string{"--spec", "-json", "../testdata/passing/"}, exit: 2, stderrHas: "-json"},
				{Desc: "summary with -json", args: []string{"summary", "-json", "../testdata/passing/"}, exit: 2, stderrHas: "-json"},
				{Desc: "an unknown --format", args: []string{"summary", "--format", "xml", "../testdata/passing/"}, exit: 2, stderrHas: `unknown --format "xml"`},
				{Desc: "an unknown --format for spec", args: []string{"spec", "--format=yaml", "../testdata/passing/"}, exit: 2, stderrHas: `unknown --format "yaml"`},
			}) {
				run, err := testkit.RunCLI(sub.Context(), ".", row.args...)
				gotest.NoError(sub, err)
				gotest.Equal(sub, row.exit, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
				gotest.Contains(sub, run.Stderr, row.stderrHas)
			}
		})
	})
}
