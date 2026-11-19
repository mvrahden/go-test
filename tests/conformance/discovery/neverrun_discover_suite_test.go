package discovery_test

import (
	"encoding/json"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// NeverRunDiscoverTestSuite: discover reports what the generator refuses as
// never run, and warns only of what the refusal leaves alone.
type NeverRunDiscoverTestSuite struct{}

func (s *NeverRunDiscoverTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second}
}

// discovered is the part of discover's document these scenarios read.
type discovered struct {
	Packages []struct {
		Broken bool              `json:"broken"`
		Suites []json.RawMessage `json:"suites"`
	} `json:"packages"`
	Warnings []struct {
		File    string `json:"file"`
		Line    int    `json:"line"`
		Col     int    `json:"col"`
		Message string `json:"message"`
	} `json:"warnings"`
}

func discoverShape(t *gotest.T, pkg string) discovered {
	run, err := testkit.RunCLI(t.Context(), ".", "discover", "../testdata/shapes/"+pkg+"/")
	gotest.NoError(t, err)
	gotest.Equal(t, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
	var out discovered
	gotest.NoError(t, json.Unmarshal([]byte(run.Stdout), &out), "stdout: %s", run.Stdout)
	gotest.Len(t, out.Packages, 1, "stdout: %s", run.Stdout)
	return out
}

func (s *NeverRunDiscoverTestSuite) TestDiscover(t *gotest.T) {
	t.When("discover reads a package declaring cases no run reaches", func(w *gotest.T) {
		w.It("reports the package broken with the refusal at the type's first such method", func(it *gotest.T) {
			out := discoverShape(it, "orphantype")
			gotest.True(it, out.Packages[0].Broken)
			gotest.Empty(it, out.Packages[0].Suites)
			gotest.NotEmpty(it, out.Warnings)
			gotest.Equal(it, "orphantype_suite_test.go", out.Warnings[0].File)
			gotest.Equal(it, 8, out.Warnings[0].Line)
			gotest.Equal(it, 21, out.Warnings[0].Col)
			gotest.Equal(it, orphanTypeRefusal, out.Warnings[0].Message)
		})
		w.It("warns of no other method of that type", func(it *gotest.T) {
			out := discoverShape(it, "orphantype")
			gotest.Len(it, out.Warnings, 1, "warnings: %v", out.Warnings)
		})
		w.It("reports a generic suite no alias instantiates broken with the refusal at its declaration", func(it *gotest.T) {
			out := discoverShape(it, "orphangeneric")
			gotest.True(it, out.Packages[0].Broken)
			gotest.Len(it, out.Warnings, 1, "warnings: %v", out.Warnings)
			gotest.Equal(it, 6, out.Warnings[0].Line)
			gotest.Regexp(it, "^generic suite OrphanContractTestSuite has no instantiation", out.Warnings[0].Message)
		})
		w.It("a Benchmark or Fuzz method on a helper a suite calls, or on a type a non-test file declares, is still warned of as never running", func(it *gotest.T) {
			out := discoverShape(it, "orphanwarned")
			gotest.False(it, out.Packages[0].Broken)
			gotest.Len(it, out.Packages[0].Suites, 1)
			var got []string
			for _, w := range out.Warnings {
				got = append(got, w.File+": "+w.Message)
			}
			gotest.Equal(it, []string{
				"loader.go: Loader.BenchmarkLoad takes *gotest.B, but Loader is not a test suite (its name must end in TestSuite): the benchmark never runs",
				"orphanwarned_suite_test.go: timer.BenchmarkTick takes *gotest.B, but timer is not a test suite (its name must end in TestSuite): the benchmark never runs",
			}, got)
		})
	})
}
