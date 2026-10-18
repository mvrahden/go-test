package conventions_test

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/mvrahden/go-test/internal/speccatalog"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SpecBindingTestSuite: specs/ states every behavior once, and each scenario
// is a conformance clause, each clause a scenario.
type SpecBindingTestSuite struct{}

func (s *SpecBindingTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

// loadBinding reads specs/ and the committed catalog and joins them.
func loadBinding(t *gotest.T) ([]speccatalog.Spec, speccatalog.Binding) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	gotest.NoError(t, err)
	js, err := os.ReadFile(filepath.Join(root, speccatalog.JSONPath))
	gotest.NoError(t, err)
	clauses, err := speccatalog.ClausesOf(js)
	gotest.NoError(t, err)
	specs, err := speccatalog.LoadSpecs(root)
	gotest.NoError(t, err)
	return specs, speccatalog.Bind(specs, clauses)
}

func (s *SpecBindingTestSuite) TestEveryScenarioIsVerified(t *gotest.T) {
	_, binding := loadBinding(t)
	t.When("each scenario in specs/ is looked up among the conformance clauses", func(w *gotest.T) {
		w.It("finds its WHEN as a When label and each THEN and AND as an It under it", func(it *gotest.T) {
			gotest.Empty(it, binding.Unverified, "reword the scenario or the conformance When/It label until they match:\n%s", strings.Join(binding.Unverified, "\n"))
		})
	})
}

func (s *SpecBindingTestSuite) TestEveryClauseIsSpecified(t *gotest.T) {
	_, binding := loadBinding(t)
	t.When("each conformance clause is looked up among the scenarios in specs/", func(w *gotest.T) {
		w.It("finds it as a WHEN with that THEN or AND", func(it *gotest.T) {
			gotest.Empty(it, binding.Unspecified, "state each clause as a scenario in specs/<capability>/spec.md:\n%s", strings.Join(binding.Unspecified, "\n"))
		})
	})
}

func (s *SpecBindingTestSuite) TestEveryTableIsStated(t *gotest.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	gotest.NoError(t, err)
	js, err := os.ReadFile(filepath.Join(root, speccatalog.JSONPath))
	gotest.NoError(t, err)
	tables, err := speccatalog.UnstatedTables(js)
	gotest.NoError(t, err)
	t.When("each conformance When that holds an Each table is inspected", func(w *gotest.T) {
		w.It("has a direct It that states what every row shows", func(it *gotest.T) {
			gotest.Empty(it, tables, "a table under a When needs a direct It that states it; wrap the loop in w.It(…):\n%s", strings.Join(tables, "\n"))
		})
	})
}

func (s *SpecBindingTestSuite) TestSpecShape(t *gotest.T) {
	specs, _ := loadBinding(t)
	var problems []string
	for i := range specs {
		problems = append(problems, specs[i].Problems()...)
	}
	t.When("each spec in specs/ is read in OpenSpec's shape", func(w *gotest.T) {
		w.It("has a purpose, requirements with SHALL or MUST, and scenarios with a WHEN and a THEN", func(it *gotest.T) {
			gotest.Empty(it, problems, "fix the spec:\n%s", strings.Join(problems, "\n"))
		})
	})
}
