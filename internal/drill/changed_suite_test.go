package drill_test

import (
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/internal/drill"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// ChangedSelectionTestSuite: which mutants a list of changed files selects.
type ChangedSelectionTestSuite struct{}

func (s *ChangedSelectionTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

const benchPatch = "Expect: gate without -count: exit code [01], want 2\n" +
	"Judge: ./tests/canary CanaryTestSuite/TestAGateNeedsSamples\n" +
	"Clause: when --gate is given with -count below 4 → exits 2 naming -count=4 as the requirement\n" +
	"--- a/cmd/gotest/bench.go\n+++ b/cmd/gotest/bench.go\n@@ -86,7 +86,7 @@\n ctx\n-old\n+new\n"

func facts() map[string]drill.MutantFacts {
	return map[string]drill.MutantFacts{
		"gate":  drill.ReadFacts(benchPatch, []drill.Run{{Pattern: "^TestCanary$", Packages: []string{"./tests/canary"}}}),
		"color": {Targets: []string{"cmd/gotest/color.go"}, Clauses: []string{"prints no color codes, even with --no-color=false"}, Packages: []string{"tests/conformance/cli"}},
	}
}

func (s *ChangedSelectionTestSuite) TestAPatchIsReadForWhatAChangeCanTouch(t *gotest.T) {
	gotest.Equal(t, drill.MutantFacts{
		Targets:  []string{"cmd/gotest/bench.go"},
		Clauses:  []string{"exits 2 naming -count=4 as the requirement"},
		Packages: []string{"tests/canary"},
	}, facts()["gate"])
}

func (s *ChangedSelectionTestSuite) TestAChangeSelectsTheMutantsItTouches(t *gotest.T) {
	names := []string{"color", "gate"}
	spec := func(path string) string {
		if path == "specs/cli/spec.md" {
			return "- **THEN** prints no color codes, even with --no-color=false\n"
		}
		return ""
	}
	for sub, tc := range gotest.Each(t, []struct {
		Name    string
		Changed []string
		Want    []string
		Note    string
	}{
		{"its own patch", []string{"tests/drill/mutants/gate.patch"}, []string{"gate"}, "drill  the changed files select 1 of 2 mutants"},
		{"the file it patches", []string{"cmd/gotest/bench.go"}, []string{"gate"}, "drill  the changed files select 1 of 2 mutants"},
		{"a test of its judge package", []string{"tests/canary/canary_suite_test.go"}, []string{"gate"}, "drill  the changed files select 1 of 2 mutants"},
		{"a fixture below its judge package", []string{"tests/conformance/cli/testdata/colors/x_suite_test.go"}, []string{"color"}, "drill  the changed files select 1 of 2 mutants"},
		{"the spec that states its clause", []string{"specs/cli/spec.md"}, []string{"color"}, "drill  the changed files select 1 of 2 mutants"},
		{"a spec that states none of its clauses", []string{"specs/lint/spec.md"}, []string{}, "drill  the changed files select 0 of 2 mutants"},
		{"a sibling whose name starts alike", []string{"tests/canaryx/a.go", "cmd/gotest/bench.go.orig"}, []string{}, "drill  the changed files select 0 of 2 mutants"},
		{"an unrelated file", []string{"README.md"}, []string{}, "drill  the changed files select 0 of 2 mutants"},
		{"nothing", nil, []string{}, "drill  the changed files select 0 of 2 mutants"},
		{"the driver", []string{"README.md", "internal/drill/plan.go"}, names, "drill  internal/drill/plan.go changed: every mutant is selected"},
		{"the module's requirements", []string{"go.sum"}, names, "drill  go.sum changed: every mutant is selected"},
	}) {
		got, note := drill.Narrow(names, tc.Changed, facts(), spec)
		gotest.Equal(sub, tc.Want, got)
		gotest.Equal(sub, tc.Note, note)
	}
}

func (s *ChangedSelectionTestSuite) TestTheChangeListIsReadFromItsFile(t *gotest.T) {
	list := filepath.Join(t.TempDir(), "changed.txt")
	gotest.NoError(t, os.WriteFile(list, []byte("cmd/gotest/bench.go\n\nREADME.md\r\n"), 0o600))

	changed, set, err := drill.ChangedFiles(map[string]string{"DRILL_CHANGED": list}, false)
	gotest.NoError(t, err)
	gotest.True(t, set)
	gotest.Equal(t, []string{"cmd/gotest/bench.go", "README.md"}, changed)

	_, set, err = drill.ChangedFiles(map[string]string{}, true)
	gotest.NoError(t, err)
	gotest.False(t, set, "named mutants without DRILL_CHANGED stay legal")

	_, _, err = drill.ChangedFiles(map[string]string{"DRILL_CHANGED": list}, true)
	gotest.ErrorContains(t, err, "DRILL_CHANGED selects the mutants itself: name mutants or set it, not both")

	_, _, err = drill.ChangedFiles(map[string]string{"DRILL_CHANGED": list + ".gone"}, false)
	gotest.ErrorContains(t, err, "DRILL_CHANGED names a file that cannot be read: "+list+".gone")
}
