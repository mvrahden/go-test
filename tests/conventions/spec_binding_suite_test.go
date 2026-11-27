package conventions_test

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/speccatalog"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SpecBindingTestSuite: specs/ states every behavior once, and each scenario
// is a conformance clause, each clause a scenario. gotest scenarios binds
// them, read from source; the rules this repository adds sit beside it.
type SpecBindingTestSuite struct {
	specs   []speccatalog.Spec
	reading speccatalog.Reading
}

func (s *SpecBindingTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	// Setup builds the CLI and runs it twice at once: just over 30s on a
	// cold Windows.
	cfg.SetupTimeout = 45 * time.Second
	return cfg
}

func (s *SpecBindingTestSuite) BeforeAll(t *gotest.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	gotest.NoError(t, err)
	s.specs, err = speccatalog.LoadSpecs(root)
	gotest.NoError(t, err)
	s.reading, err = speccatalog.Read(t.Context(), root)
	gotest.NoError(t, err)
}

func (s *SpecBindingTestSuite) AfterAll(t *gotest.T) {}

func (s *SpecBindingTestSuite) TestEveryScenarioIsBound(t *gotest.T) {
	doc := &s.reading.Scenarios
	t.When("gotest scenarios binds specs/ to the conformance suites", func(w *gotest.T) {
		w.It("binds every scenario, each THEN and AND to one It under a When its WHEN labels", func(it *gotest.T) {
			gotest.NotEmpty(it, doc.Scenarios)
			gotest.Empty(it, speccatalog.NotBound(doc), "reword the scenario or the conformance When/It label until they match:\n%s", speccatalog.Report(doc))
		})
		w.It("finds no It under a bound When that no clause states", func(it *gotest.T) {
			gotest.Empty(it, doc.Unstated, "state each It as a THEN or AND of the scenario, or move it under another When:\n%s", speccatalog.Report(doc))
		})
		w.It("lists every behavior of the suites, none incomplete", func(it *gotest.T) {
			gotest.Empty(it, doc.Incomplete, "write the behaviors so the static reading lists them: literal labels and tables")
		})
	})
}

func (s *SpecBindingTestSuite) TestEveryClauseIsSpecified(t *gotest.T) {
	unbound := speccatalog.UnboundWhens(&s.reading)
	t.When("each conformance When with an It under it is looked up among the scenarios in specs/", func(w *gotest.T) {
		w.It("finds a scenario whose WHEN binds it", func(it *gotest.T) {
			gotest.Empty(it, unbound, "state each clause as a scenario in specs/<capability>/spec.md:\n%s", strings.Join(unbound, "\n"))
		})
	})
}

func (s *SpecBindingTestSuite) TestEveryScenarioBindsItsOwnCapability(t *gotest.T) {
	outside := speccatalog.OutsideCapability(&s.reading.Scenarios)
	t.When("the Its a scenario binds are located", func(w *gotest.T) {
		w.It("lie in the conformance package of the scenario's capability", func(it *gotest.T) {
			gotest.Empty(it, outside, "specs/<capability>/spec.md is verified by tests/conformance/<capability>; move the suite or the scenario:\n%s", strings.Join(outside, "\n"))
		})
	})
}

func (s *SpecBindingTestSuite) TestEveryClauseIsLabeledAsWritten(t *gotest.T) {
	inexact := speccatalog.Inexact(&s.reading.Scenarios)
	t.When("the labels a scenario binds are compared with its text", func(w *gotest.T) {
		w.It("equal its WHEN and each THEN and AND as written", func(it *gotest.T) {
			gotest.Empty(it, inexact, "spell the label as the spec does:\n%s", strings.Join(inexact, "\n"))
		})
	})
}

func (s *SpecBindingTestSuite) TestEveryTableIsStated(t *gotest.T) {
	tables := speccatalog.UnstatedTables(&s.reading)
	t.When("each conformance When that holds an Each table is inspected", func(w *gotest.T) {
		w.It("has a direct It that states what every row shows", func(it *gotest.T) {
			gotest.Empty(it, tables, "a table under a When needs a direct It that states it; wrap the loop in w.It(…):\n%s", strings.Join(tables, "\n"))
		})
	})
}

func (s *SpecBindingTestSuite) TestEveryItIsDirectlyUnderAWhen(t *gotest.T) {
	misplaced := speccatalog.MisplacedIts(&s.reading)
	t.When("each conformance It is placed in its tree", func(w *gotest.T) {
		w.It("sits directly under a When, so a scenario can bind to it", func(it *gotest.T) {
			gotest.Empty(it, misplaced, "a clause is an It directly under a When; an It under Each rows, under another It or with no When above it binds to no scenario:\n%s", strings.Join(misplaced, "\n"))
		})
	})
}

func (s *SpecBindingTestSuite) TestSpecShape(t *gotest.T) {
	var problems []string
	for i := range s.specs {
		problems = append(problems, s.specs[i].Problems()...)
	}
	t.When("each spec in specs/ is read in OpenSpec's shape", func(w *gotest.T) {
		w.It("has a purpose, requirements with SHALL or MUST, and scenarios with a WHEN and a THEN", func(it *gotest.T) {
			gotest.Empty(it, problems, "fix the spec:\n%s", strings.Join(problems, "\n"))
		})
	})
}
