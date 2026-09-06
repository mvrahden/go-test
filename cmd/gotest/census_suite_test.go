package main_test

import (
	"bytes"
	"strings"

	main "github.com/mvrahden/go-test/cmd/gotest"
	"github.com/mvrahden/go-test/internal/gotestspec"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CensusTestSuite covers the guard against green-by-absence: every declared
// test method must have produced a verdict before a green run is believed.
type CensusTestSuite struct{}

func (s *CensusTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

type censusCtx struct{}

func (s *CensusTestSuite) BeforeEach(t *gotest.T) *censusCtx { return &censusCtx{} }

func parseStream(t *gotest.T, stream string) []gotestspec.TestEvent {
	events, err := gotestspec.ParseEvents(strings.NewReader(stream))
	gotest.NoError(t, err)
	return events
}

const greenStream = `{"Action":"run","Package":"example.com/pkg","Test":"TestCartTestSuite"}
{"Action":"run","Package":"example.com/pkg","Test":"TestCartTestSuite/TestAdd"}
{"Action":"run","Package":"example.com/pkg","Test":"TestCartTestSuite/TestAdd/it_adds"}
{"Action":"pass","Package":"example.com/pkg","Test":"TestCartTestSuite/TestAdd/it_adds"}
{"Action":"pass","Package":"example.com/pkg","Test":"TestCartTestSuite/TestAdd"}
{"Action":"run","Package":"example.com/pkg","Test":"TestCartTestSuite/TestRemove"}
{"Action":"skip","Package":"example.com/pkg","Test":"TestCartTestSuite/TestRemove"}
{"Action":"pass","Package":"example.com/pkg","Test":"TestCartTestSuite"}
{"Action":"pass","Package":"example.com/pkg"}
`

func (s *CensusTestSuite) TestExecutedCasesReadTheStream(t *gotest.T, _ *censusCtx) {
	t.It("lists every Suite/Method with a terminal verdict, skip included, behaviors excluded", func(it *gotest.T) {
		got := main.ExportExecutedCases(parseStream(it, greenStream))
		gotest.Equal(it, []main.ExportCensusCase{
			{Pkg: "example.com/pkg", Path: "TestCartTestSuite/TestAdd"},
			{Pkg: "example.com/pkg", Path: "TestCartTestSuite/TestRemove"},
		}, got)
	})

	t.It("counts a suite skipped as a whole as covering its methods", func(it *gotest.T) {
		stream := `{"Action":"run","Package":"example.com/pkg","Test":"TestGuardedTestSuite"}
{"Action":"skip","Package":"example.com/pkg","Test":"TestGuardedTestSuite"}
{"Action":"pass","Package":"example.com/pkg"}
`
		declared := []main.ExportCensusCase{{Pkg: "example.com/pkg", Path: "TestGuardedTestSuite/TestX"}}
		missing := main.ExportCensusMissing(declared, main.ExportExecutedCases(parseStream(it, stream)))
		gotest.Empty(it, missing)
	})
}

func (s *CensusTestSuite) TestMissingIsDeclaredMinusExecuted(t *gotest.T, _ *censusCtx) {
	declared := []main.ExportCensusCase{
		{Pkg: "example.com/pkg", Path: "TestCartTestSuite/TestAdd"},
		{Pkg: "example.com/pkg", Path: "TestCartTestSuite/TestRemove"},
		{Pkg: "example.com/pkg", Path: "TestCartTestSuite/TestTotal"},
	}
	executed := main.ExportExecutedCases(parseStream(t, greenStream))

	t.It("reports the method that never ran, in declaration order", func(it *gotest.T) {
		gotest.Equal(it, []main.ExportCensusCase{{Pkg: "example.com/pkg", Path: "TestCartTestSuite/TestTotal"}},
			main.ExportCensusMissing(declared, executed))
	})

	t.It("keeps packages apart: a name executed elsewhere does not count", func(it *gotest.T) {
		other := []main.ExportCensusCase{{Pkg: "example.com/other", Path: "TestCartTestSuite/TestAdd"}}
		gotest.Equal(it, other, main.ExportCensusMissing(other, executed))
	})
}

func (s *CensusTestSuite) TestEnforcementRules(t *gotest.T, _ *censusCtx) {
	declared := []main.ExportCensusCase{{Pkg: "example.com/pkg", Path: "TestCartTestSuite/TestTotal"}}
	executed := main.ExportExecutedCases(parseStream(t, greenStream))

	t.When("a green run is missing a declared test", func(w *gotest.T) {
		var out bytes.Buffer
		code := main.ExportEnforceCensus(&out, 0, nil, declared, executed)

		w.It("exits 2 and names the test", func(it *gotest.T) {
			gotest.Equal(it, 2, code)
			gotest.Contains(it, out.String(), "FAIL: census: 1 declared test(s) never ran")
			gotest.Contains(it, out.String(), "example.com/pkg TestCartTestSuite/TestTotal")
		})
	})

	t.When("the run is already red", func(w *gotest.T) {
		var out bytes.Buffer
		code := main.ExportEnforceCensus(&out, 1, nil, declared, executed)

		w.It("passes the verdict through untouched and prints nothing", func(it *gotest.T) {
			gotest.Equal(it, 1, code)
			gotest.Empty(it, out.String())
		})
	})

	t.When("the run was filtered", func(w *gotest.T) {
		for sub, args := range gotest.Each(w, [][]string{{"-run", "TestX"}, {"-run=TestX"}, {"-skip=Slow"}, {"-list", ".*"}, {"-test.run=X"}}) {
			var out bytes.Buffer
			code := main.ExportEnforceCensus(&out, 0, args, declared, executed)
			gotest.Equal(sub, 0, code)
			gotest.Contains(sub, out.String(), "note: census skipped")
		}
	})

	t.When("nothing is missing", func(w *gotest.T) {
		var out bytes.Buffer
		code := main.ExportEnforceCensus(&out, 0, []string{"-race", "-count=2"}, executed, executed)

		w.It("stays silent and green", func(it *gotest.T) {
			gotest.Equal(it, 0, code)
			gotest.Empty(it, out.String())
		})
	})
}

func (s *CensusTestSuite) TestRunsAreCensused(t *gotest.T, _ *censusCtx) {
	// Real pipeline runs over tiny fixture packages. A whole-suite skip and a
	// filtered run must stay green; a misjudged census would turn either into 2.
	t.When("a suite skips itself in BeforeAll", func(w *gotest.T) {
		w.It("summary stays green: the skip covers the suite's methods", func(it *gotest.T) {
			gotest.Equal(it, 0, main.ExportRunSummary(main.Invocation{Args: []string{"./testdata/census/skipped/"}}))
		})
	})

	t.When("the run is filtered to nothing", func(w *gotest.T) {
		w.It("spec stands down instead of reporting every method missing", func(it *gotest.T) {
			gotest.Equal(it, 0, main.ExportRunSpec(main.Invocation{Args: []string{"./testdata/census/plain/", "-run", "TestNothingMatches"}}))
		})
	})

	t.When("a plain package runs in -json mode", func(w *gotest.T) {
		w.It("is censused through the observer and stays green", func(it *gotest.T) {
			gotest.Equal(it, 0, main.Run(main.ExecConfig{PackagePatterns: []string{"./testdata/census/plain/"}, JSON: true}))
		})
	})
}
