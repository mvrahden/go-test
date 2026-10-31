package lint_test

import (
	"github.com/mvrahden/go-test/internal/lint"
	"github.com/mvrahden/go-test/pkg/gotest"
	"golang.org/x/tools/go/analysis/analysistest"
)

// GenerationRejectsTestSuite covers the generation-rejects rule and the
// go test name rule the bench and fuzz rules classify methods by.
type GenerationRejectsTestSuite struct{}

func (s *GenerationRejectsTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *GenerationRejectsTestSuite) TestGenerationRejects(t *gotest.T) {
	t.When("a suite holds what the generator rejects", func(w *gotest.T) {
		w.It("reports each rejection with the generator's message, except on a line another rule reported", func(it *gotest.T) {
			analysistest.Run(it.T(), analysistest.TestData(), lint.Analyzer, "withgeneration")
		})
	})
	t.When("a type named like a suite declares no case", func(w *gotest.T) {
		w.It("reports it at its declaration, and not a suite whose only case is excluded", func(it *gotest.T) {
			analysistest.Run(it.T(), analysistest.TestData(), lint.Analyzer, "withcaseless")
		})
	})
}

func (s *GenerationRejectsTestSuite) TestMethodNames(t *gotest.T) {
	t.When("a method continues its prefix with a lowercase letter", func(w *gotest.T) {
		w.It("is no benchmark", func(it *gotest.T) {
			gotest.False(it, lint.ExportIsBenchmarkMethodName("Benchmarked"))
			gotest.True(it, lint.ExportIsBenchmarkMethodName("BenchmarkParse"))
			gotest.True(it, lint.ExportIsBenchmarkMethodName("X_Benchmark_parse"))
		})
		w.It("is no fuzz target", func(it *gotest.T) {
			gotest.False(it, lint.ExportIsFuzzMethodName("Fuzzy"))
			gotest.True(it, lint.ExportIsFuzzMethodName("FuzzParse"))
			gotest.True(it, lint.ExportIsFuzzMethodName("F_Fuzz_parse"))
		})
	})
}
