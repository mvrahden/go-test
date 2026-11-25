package scenarios_test

import (
	"os"
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// ScenariosReadingTestSuite: which spec files scenarios reads, and how it
// reads their scenarios.
type ScenariosReadingTestSuite struct {
	dir                             string
	plain, variant, delta, fromConf testkit.Run
	byDefault, none                 testkit.Run
}

func (s *ScenariosReadingTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second, SetupTimeout: 60 * time.Second}
}

func (s *ScenariosReadingTestSuite) BeforeAll(t *gotest.T) {
	s.dir = tempDir(t, "gotest-scenarios-reading-")
	mod := stage(t, s.dir, "mod")
	s.byDefault = scenarios(t, mod, "--format=json")
	s.plain = scenarios(t, mod, "--specs=forms/plain", "--format=json")
	s.variant = scenarios(t, mod, "--specs=forms/variant", "--format=json")
	s.delta = scenarios(t, mod, "--specs=openspec/changes/add-gift/specs", "--format=json")

	conf := stage(t, s.dir, "conf")
	gotest.NoError(t, os.WriteFile(filepath.Join(conf, ".gotest.yml"), []byte("scenarios:\n  specs: [forms/plain]\n"), 0o600))
	s.fromConf = scenarios(t, conf, "--format=json")

	empty := filepath.Join(s.dir, "empty")
	gotest.NoError(t, os.MkdirAll(empty, 0o750))
	gotest.NoError(t, os.WriteFile(filepath.Join(empty, "go.mod"), []byte("module empty\n\ngo 1.26.0\n"), 0o600))
	s.none = scenarios(t, empty)
}

func (s *ScenariosReadingTestSuite) AfterAll(t *gotest.T) {
	if s.dir != "" {
		gotest.NoError(t, os.RemoveAll(s.dir))
	}
}

func (s *ScenariosReadingTestSuite) TestReadingTheSpecFiles(t *gotest.T) {
	t.When("scenarios runs in a module with spec files under specs and openspec/specs and no --specs", func(w *gotest.T) {
		w.It("reads the scenarios of both, each under the capability its directory names", func(it *gotest.T) {
			gotest.Equal(it, 0, s.byDefault.Exit, "stderr: %s", s.byDefault.Stderr)
			doc := readDoc(it, &s.byDefault)
			gotest.Equal(it, []string{"openspec/specs/stock/spec.md", "specs/cart/spec.md", "specs/checkout/spec.md"}, doc.Specs)
			caps := map[string]string{}
			for i := range doc.Scenarios {
				caps[doc.Scenarios[i].File] = doc.Scenarios[i].Capability
			}
			gotest.Equal(it, map[string]string{"openspec/specs/stock/spec.md": "stock", "specs/cart/spec.md": "cart", "specs/checkout/spec.md": "checkout"}, caps)
			gotest.Equal(it, 18, doc.Summary.Scenarios)
		})
	})

	t.When("a spec writes its clauses bold, bold with a colon, underlined, bare in capitals and wrapped onto a second line", func(w *gotest.T) {
		type step struct{ Keyword, Text string }
		read := func(it *gotest.T, run *testkit.Run) (titles []string, steps []step) {
			doc := readDoc(it, run)
			for i := range doc.Scenarios {
				sc := &doc.Scenarios[i]
				titles = append(titles, sc.Title)
				for _, c := range append(sc.Conditions, sc.Clauses...) {
					steps = append(steps, step{c.Keyword, c.Text})
				}
			}
			return titles, steps
		}
		w.It("reads the same clauses as from the plain bold form", func(it *gotest.T) {
			_, plain := read(it, &s.plain)
			_, variant := read(it, &s.variant)
			gotest.Equal(it, []step{
				{"GIVEN", "a member"}, {"WHEN", "a discount applies"}, {"THEN", "the total drops"},
				{"AND", "the receipt shows the discount"}, {"AND", "the member is told"},
			}, plain)
			gotest.Equal(it, plain, variant)
		})
		w.It("reads no heading or clause inside a code fence", func(it *gotest.T) {
			titles, _ := read(it, &s.variant)
			gotest.Equal(it, []string{"Every form"}, titles)
		})
	})

	t.When("--specs names a change's specs directory", func(w *gotest.T) {
		w.It("reads its ADDED and MODIFIED requirements and none of its REMOVED ones", func(it *gotest.T) {
			doc := readDoc(it, &s.delta)
			var got []string
			for i := range doc.Scenarios {
				got = append(got, doc.Scenarios[i].Requirement+" / "+doc.Scenarios[i].Title)
			}
			gotest.Equal(it, []string{"Gift wrap / A wrapped gift", "Totals / A total with wrapping"}, got)
		})
	})

	t.When(".gotest.yml names a directory under scenarios.specs and no --specs is given", func(w *gotest.T) {
		w.It("reads the spec files of that directory and no other", func(it *gotest.T) {
			doc := readDoc(it, &s.fromConf)
			gotest.Equal(it, []string{"forms/plain/spec.md"}, doc.Specs)
		})
	})

	t.When("scenarios finds no spec file", func(w *gotest.T) {
		w.It("exits 2 naming the places it looked", func(it *gotest.T) {
			gotest.Equal(it, 2, s.none.Exit)
			gotest.Empty(it, s.none.Stdout)
			gotest.Contains(it, s.none.Stderr, "no spec.md under specs, openspec/specs")
		})
	})
}
