package fuzz_test

import (
	"os"
	"strconv"
	"strings"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// FuzzSeedTypeTestSuite: a seed of the wrong type fails its target at the
// f.Add call.
type FuzzSeedTypeTestSuite struct{}

func (s *FuzzSeedTypeTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *FuzzSeedTypeTestSuite) TestWrongType(t *gotest.T) {
	const file = "fzseedtype_suite_test.go"
	src, err := os.ReadFile("../testdata/fzseedtype/" + file)
	gotest.NoError(t, err)
	line := 0
	for i, l := range strings.Split(string(src), "\n") {
		if strings.Contains(l, "f.Add(int64(5))") {
			line = i + 1
		}
	}
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/fzseedtype/")
	gotest.NoError(t, err)
	var output strings.Builder
	for _, ev := range run.Events {
		output.WriteString(ev.Output)
	}
	t.When("an f.Add seed's type does not match the fuzz target's parameters", func(w *gotest.T) {
		w.It("the failure is located at the f.Add call", func(it *gotest.T) {
			gotest.Equal(it, "fail", run.Verdict("FuzzSeedTypeTestSuite_FuzzCount"), "stdout: %s", run.Stdout)
			gotest.Contains(it, output.String(), file+":"+strconv.Itoa(line)+": seed #1: value 1: f.Add was given int64, but this fuzz target takes int")
		})
	})
}
