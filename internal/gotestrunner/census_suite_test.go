package gotestrunner_test

import (
	"context"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/gotestspec"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CensusTestSuite covers the guard against green-by-absence: a green run that
// ran to completion is believed only when every declared unit has a verdict.
type CensusTestSuite struct{}

func (s *CensusTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

type unit = gotestrunner.CensusCase

const censusPkg = "example.com/pkg"

const censusGreenStream = `{"Action":"run","Package":"example.com/pkg","Test":"TestCartTestSuite"}
{"Action":"run","Package":"example.com/pkg","Test":"TestCartTestSuite/TestAdd"}
{"Action":"run","Package":"example.com/pkg","Test":"TestCartTestSuite/TestAdd/it_adds"}
{"Action":"pass","Package":"example.com/pkg","Test":"TestCartTestSuite/TestAdd/it_adds"}
{"Action":"pass","Package":"example.com/pkg","Test":"TestCartTestSuite/TestAdd"}
{"Action":"run","Package":"example.com/pkg","Test":"TestCartTestSuite/TestRemove"}
{"Action":"skip","Package":"example.com/pkg","Test":"TestCartTestSuite/TestRemove"}
{"Time":"2026-09-13T10:00:01.5Z","Action":"pass","Package":"example.com/pkg","Test":"TestCartTestSuite","Elapsed":1.5}
{"Time":"2026-09-13T10:00:04.21Z","Action":"pass","Package":"example.com/pkg","Elapsed":4.21}
`

const censusFuzzStream = `{"Action":"run","Package":"example.com/pkg","Test":"FuzzCartTestSuite_FuzzTrim"}
{"Action":"run","Package":"example.com/pkg","Test":"FuzzCartTestSuite_FuzzTrim/seed#0"}
{"Action":"pass","Package":"example.com/pkg","Test":"FuzzCartTestSuite_FuzzTrim/seed#0"}
{"Action":"pass","Package":"example.com/pkg","Test":"FuzzCartTestSuite_FuzzTrim"}
{"Action":"run","Package":"example.com/pkg","Test":"FuzzCartTestSuite_FuzzStarted"}
{"Action":"pass","Package":"example.com/pkg"}
`

const censusBenchStream = `{"Action":"run","Package":"example.com/pkg","Test":"BenchmarkCacheTestSuite"}
{"Action":"run","Package":"example.com/pkg","Test":"BenchmarkCacheTestSuite/BenchmarkGetHit"}
{"Action":"output","Package":"example.com/pkg","Test":"BenchmarkCacheTestSuite/BenchmarkGetHit","Output":"BenchmarkCacheTestSuite/BenchmarkGetHit\n"}
{"Action":"output","Package":"example.com/pkg","Test":"BenchmarkCacheTestSuite/BenchmarkGetHit","Output":"BenchmarkCacheTestSuite/BenchmarkGetHit-6   \t 1\t 260.0 ns/op\n"}
{"Action":"run","Package":"example.com/pkg","Test":"BenchmarkCacheTestSuite/BenchmarkStarted"}
{"Action":"run","Package":"example.com/pkg","Test":"BenchmarkCacheTestSuite/BenchmarkBroken"}
{"Action":"fail","Package":"example.com/pkg","Test":"BenchmarkCacheTestSuite/BenchmarkBroken"}
{"Action":"pass","Package":"example.com/pkg"}
`

// takeCensus runs the census of a green capturing test run over stream.
func takeCensus(stream string, declared gotestrunner.DeclaredUnits, args []string) (code int, stderr, booked string) {
	return gotestrunner.ExportCensus(gotestrunner.RunCaptureJSON, stream, declared, args, false, 0, nil)
}

// censusTree builds the tree of stream and returns its example.com/pkg package.
func censusTree(t *gotest.T, stream string) *gotestspec.Package {
	events, err := gotestspec.ParseEvents(strings.NewReader(stream))
	gotest.NoError(t, err)
	for _, p := range gotestspec.BuildTree(events) {
		if p.Path == censusPkg {
			return p
		}
	}
	return nil
}

func (s *CensusTestSuite) TestExecutedUnitsReadTheStream(t *gotest.T) {
	t.It("lists every Suite/Method with a verdict, skip included, behaviors excluded", func(it *gotest.T) {
		tests, _, _ := gotestrunner.ExportExecuted(censusGreenStream)
		gotest.Equal(it, []unit{
			{Pkg: censusPkg, Path: "TestCartTestSuite/TestAdd"},
			{Pkg: censusPkg, Path: "TestCartTestSuite/TestRemove"},
		}, tests)
	})

	t.It("reads a line split across writes", func(it *gotest.T) {
		whole, _, _ := gotestrunner.ExportExecuted(censusGreenStream)
		split, _, _ := gotestrunner.ExportExecuted(censusGreenStream[:250], censusGreenStream[250:])
		gotest.Equal(it, whole, split)
	})

	t.It("counts a fuzz wrapper with a verdict, not its seeds, not one that merely started, and not as a method", func(it *gotest.T) {
		tests, fuzz, _ := gotestrunner.ExportExecuted(censusFuzzStream)
		gotest.Empty(it, tests)
		gotest.Equal(it, []unit{{Pkg: censusPkg, Path: "FuzzCartTestSuite_FuzzTrim"}}, fuzz)
	})

	t.It("counts a benchmark with a result line or a failure, not one that merely started", func(it *gotest.T) {
		_, _, benchmarks := gotestrunner.ExportExecuted(censusBenchStream)
		gotest.Equal(it, []unit{
			{Pkg: censusPkg, Path: "BenchmarkCacheTestSuite/BenchmarkGetHit"},
			{Pkg: censusPkg, Path: "BenchmarkCacheTestSuite/BenchmarkBroken"},
		}, benchmarks)
	})
}

func (s *CensusTestSuite) TestEnforcementRules(t *gotest.T) {
	declared := gotestrunner.DeclaredUnits{Tests: []unit{
		{Pkg: censusPkg, Path: "TestCartTestSuite/TestAdd"},
		{Pkg: censusPkg, Path: "TestCartTestSuite/TestTotal"},
		{Pkg: "example.com/other", Path: "TestCartTestSuite/TestAdd"},
	}}

	t.When("a green run is missing declared tests", func(w *gotest.T) {
		code, stderr, _ := takeCensus(censusGreenStream, declared, nil)

		w.It("exits 2 and names each one in declaration order, keeping packages apart", func(it *gotest.T) {
			gotest.Equal(it, 2, code)
			gotest.Contains(it, stderr, "FAIL: census: 2 declared test(s) never ran\n  example.com/pkg TestCartTestSuite/TestTotal\n  example.com/other TestCartTestSuite/TestAdd\n")
		})
	})

	t.When("a suite skipped itself as a whole", func(w *gotest.T) {
		code, _, _ := takeCensus(`{"Action":"run","Package":"example.com/pkg","Test":"TestGuardedTestSuite"}
{"Action":"skip","Package":"example.com/pkg","Test":"TestGuardedTestSuite"}
{"Action":"pass","Package":"example.com/pkg"}
`, gotestrunner.DeclaredUnits{Tests: []unit{{Pkg: censusPkg, Path: "TestGuardedTestSuite/TestX"}}}, nil)

		w.It("counts the skip for its methods", func(it *gotest.T) {
			gotest.Equal(it, 0, code)
		})
	})

	t.When("a green run is missing a declared fuzz target", func(w *gotest.T) {
		code, stderr, _ := takeCensus(censusFuzzStream, gotestrunner.DeclaredUnits{Fuzz: []unit{
			{Pkg: censusPkg, Path: "FuzzCartTestSuite_FuzzTrim"},
			{Pkg: censusPkg, Path: "FuzzCartTestSuite_FuzzStarted"},
		}}, nil)

		w.It("exits 2 and names the target with its own wording", func(it *gotest.T) {
			gotest.Equal(it, 2, code)
			gotest.Contains(it, stderr, "FAIL: census: 1 declared fuzz target(s) never ran\n  example.com/pkg FuzzCartTestSuite_FuzzStarted\n")
			gotest.NotContains(it, stderr, "test(s) never ran")
		})
	})

	t.When("nothing is missing", func(w *gotest.T) {
		code, stderr, booked := takeCensus(censusGreenStream, gotestrunner.DeclaredUnits{Tests: declared.Tests[:1]}, []string{"-race", "-count=2"})

		w.It("stays green and silent and books nothing", func(it *gotest.T) {
			gotest.Equal(it, 0, code)
			gotest.Empty(it, stderr)
			gotest.Empty(it, booked)
		})
	})

	t.When("the run was filtered", func(w *gotest.T) {
		for sub, args := range gotest.Each(w, [][]string{{"-run", "TestX"}, {"-run=TestX"}, {"-skip=Slow"}, {"-list", ".*"}, {"-test.run=X"}}) {
			code, stderr, booked := takeCensus(censusGreenStream, declared, args)
			gotest.Equal(sub, 0, code)
			gotest.Contains(sub, stderr, "note: census skipped")
			gotest.Empty(sub, booked)
		}
	})

	t.When("the census cannot judge the run", func(w *gotest.T) {
		for sub, tc := range gotest.Each(w, []struct { //nolint:gocritic // rangeValCopy: intentional
			Desc string
			mode gotestrunner.RunMode
			code int
			err  error
		}{
			{Desc: "the run is already red", mode: gotestrunner.RunCaptureJSON, code: 1},
			{Desc: "an interrupt cut it short", mode: gotestrunner.RunCaptureJSON, code: 130, err: context.Canceled},
			{Desc: "its deadline cut it short", mode: gotestrunner.RunCaptureJSON, err: context.DeadlineExceeded},
			{Desc: "the text run keeps no event stream", mode: gotestrunner.RunBatchText},
		}) {
			code, stderr, booked := gotestrunner.ExportCensus(tc.mode, censusGreenStream, declared, nil, false, tc.code, tc.err)
			gotest.Equal(sub, tc.code, code)
			gotest.Empty(sub, stderr)
			gotest.Empty(sub, booked)
		}
	})
}

func (s *CensusTestSuite) TestBenchRuns(t *gotest.T) {
	declared := gotestrunner.DeclaredUnits{Benchmarks: []unit{
		{Pkg: censusPkg, Path: "BenchmarkCacheTestSuite/BenchmarkGetHit"},
		{Pkg: censusPkg, Path: "BenchmarkCacheTestSuite/BenchmarkStarted"},
	}}

	t.When("a bench run is missing a declared benchmark", func(w *gotest.T) {
		code, stderr, _ := gotestrunner.ExportCensus(gotestrunner.RunCaptureJSON, censusBenchStream, declared, nil, true, 0, nil)

		w.It("exits 2 and names it with its own wording", func(it *gotest.T) {
			gotest.Equal(it, 2, code)
			gotest.Contains(it, stderr, "FAIL: census: 1 declared benchmark(s) never ran\n  example.com/pkg BenchmarkCacheTestSuite/BenchmarkStarted\n")
		})
	})

	t.When("the caller selects benchmarks or suites", func(w *gotest.T) {
		for sub, args := range gotest.Each(w, [][]string{{"-bench=^X$"}, {"-bench", "."}, {"-run=X"}}) {
			code, stderr, _ := gotestrunner.ExportCensus(gotestrunner.RunCaptureJSON, censusBenchStream, declared, args, true, 0, nil)
			gotest.Equal(sub, 0, code)
			gotest.Contains(sub, stderr, "note: census skipped")
		}
	})

	t.When("a test run meets declared benchmarks", func(w *gotest.T) {
		code, _, _ := takeCensus(censusGreenStream, declared, nil)

		w.It("leaves them to the not-run note", func(it *gotest.T) {
			gotest.Equal(it, 0, code)
		})
	})
}

func (s *CensusTestSuite) TestMissingUnitsAreBookedIntoTheStream(t *gotest.T) {
	t.When("units are missing", func(w *gotest.T) {
		_, _, booked := takeCensus(censusGreenStream, gotestrunner.DeclaredUnits{
			Tests: []unit{{Pkg: censusPkg, Path: "TestCartTestSuite/TestTotal"}},
			Fuzz:  []unit{{Pkg: "example.com/other", Path: "FuzzCartTestSuite_FuzzStarted"}},
		}, nil)

		w.It("books each as a failed test with the census as its output, then fails its suite and package with their own time and duration", func(it *gotest.T) {
			gotest.Equal(it, `{"Action":"run","Package":"example.com/pkg","Test":"TestCartTestSuite/TestTotal"}
{"Action":"output","Package":"example.com/pkg","Test":"TestCartTestSuite/TestTotal","Output":"    census: declared test never ran\n"}
{"Action":"fail","Package":"example.com/pkg","Test":"TestCartTestSuite/TestTotal"}
{"Time":"2026-09-13T10:00:01.5Z","Action":"fail","Package":"example.com/pkg","Test":"TestCartTestSuite","Elapsed":1.5}
{"Action":"run","Package":"example.com/other","Test":"FuzzCartTestSuite_FuzzStarted"}
{"Action":"output","Package":"example.com/other","Test":"FuzzCartTestSuite_FuzzStarted","Output":"    census: declared fuzz target never ran\n"}
{"Action":"fail","Package":"example.com/other","Test":"FuzzCartTestSuite_FuzzStarted"}
{"Time":"2026-09-13T10:00:04.21Z","Action":"fail","Package":"example.com/pkg","Elapsed":4.21}
{"Action":"fail","Package":"example.com/other"}
`, booked)
		})

		w.It("turns the green tree red at the method, the suite and the package, keeping their durations", func(it *gotest.T) {
			pkg := censusTree(it, censusGreenStream+booked)
			gotest.NotZero(it, pkg)
			gotest.Equal(it, gotestspec.StatusFail, pkg.Status)
			gotest.Equal(it, 4210*time.Millisecond, pkg.Duration)
			var suite, total *gotestspec.Node
			for _, n := range pkg.Nodes {
				if n.Name == "TestCartTestSuite" {
					suite = n
				}
				for _, m := range n.Children {
					if m.Name == "TestTotal" {
						total = m
					}
				}
			}
			gotest.NotZero(it, suite)
			gotest.Equal(it, gotestspec.StatusFail, suite.Status)
			gotest.Equal(it, 1500*time.Millisecond, suite.Duration)
			gotest.NotZero(it, total)
			gotest.Equal(it, gotestspec.StatusFail, total.Status)
			gotest.Contains(it, total.Output, "    census: declared test never ran\n")
		})
	})

	t.When("a fuzz target of a suite that ran is missing", func(w *gotest.T) {
		_, _, booked := takeCensus(censusGreenStream, gotestrunner.DeclaredUnits{
			Fuzz: []unit{{Pkg: censusPkg, Path: "FuzzCartTestSuite_FuzzTrim"}},
		}, nil)

		w.It("fails the suite the tree nests the fuzz target under", func(it *gotest.T) {
			gotest.Contains(it, booked, `{"Time":"2026-09-13T10:00:01.5Z","Action":"fail","Package":"example.com/pkg","Test":"TestCartTestSuite","Elapsed":1.5}`)
			pkg := censusTree(it, censusGreenStream+booked)
			gotest.NotZero(it, pkg)
			var suite *gotestspec.Node
			for _, n := range pkg.Nodes {
				if n.Name == "TestCartTestSuite" {
					suite = n
				}
			}
			gotest.NotZero(it, suite)
			gotest.Equal(it, gotestspec.StatusFail, suite.Status)
		})
	})
}

func (s *CensusTestSuite) TestBenchmarksNotRunNote(t *gotest.T) {
	note := func(args []string, declared int) string {
		var out strings.Builder
		gotestrunner.NoteBenchmarksNotRun(&out, args, declared)
		return out.String()
	}

	t.It("names the count when a test run leaves declared benchmarks unexecuted", func(it *gotest.T) {
		gotest.Equal(it, "note: 3 benchmark(s) not run — gotest runs tests; use 'gotest bench'\n", note(nil, 3))
	})

	t.It("stays silent when -bench selected them: the run was asked to run benchmarks", func(it *gotest.T) {
		gotest.Empty(it, note([]string{"-bench=."}, 3))
	})

	t.It("stays silent under -run: a filtered run makes no claim about the rest", func(it *gotest.T) {
		gotest.Empty(it, note([]string{"-run", "TestAdd"}, 3))
	})

	t.It("stays silent when the packages declare none", func(it *gotest.T) {
		gotest.Empty(it, note(nil, 0))
	})
}
